package cmd

import (
	"regexp"
	"strconv"
	"fmt"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2"
	"time"
	"github.com/bvinc/go-sqlite-lite/sqlite3"
	"os"
	"strings"
	"log"
    "io/ioutil"
    "gopkg.in/yaml.v2"
    "github.com/gorilla/sessions"
)

//Command -
type Command struct {
    Name string
    Path string
}
//LogFile -
type LogFile struct {
    Name string //Must be unique within a host running this app. Used to save the tail pos
    Paths []string
    Timelayout string //Parse the match below into go time object
    Timepattern string //extract the timestamp part into a timeStr which is fed into the Timelayout
    Timeadjust string //If the time extracted string miss some info (like year or zone etc) this string will be appended to the string. If it contains a golang timelayout token with one extra space at the end of the string (eg. '2004 ') then these token will be parsed as the current for example year.
    Timestrreplace []string //Do search/replace the capture before parse time. As go does not support , aas sec fraction this is to work around for this case.
    Pattern string //will be matched to extract the HOSTNAME APP-NAME MSG part of the line.
    Multilineptn string //detect if the line is part of the previous line
    Excludepatterns []string //If log line match this pattern it will be excluded
    Includepatterns []string
    Appname string //Overrite the appname of the logfile if not empty
}
//AppConfig -
type AppConfig struct { //Why do I have to tag every field! Because yaml driver automatically lowercase the field name to look into the yaml file <yuk>
    Serverdomain string
    Port int //Server mode - Port to listen for web gui, log harvesting and Command executor
    Commands []Command
    JwtKey string
    Logfiles []LogFile
    Serverurl string //Client mode - URL to ship log to or to send command to be executed
    Logdbpath string
    Dbtimeout string
    Sslcert string
    Sslkey string
    PasswordFilterPatterns []string `yaml:"passwordfilterpatterns"`
    AppGoogleClientID string
    AppGoogleClientSecret string
    Sessionkey string
    AuthorizedDomain map[string]bool
    AWSLogs []AWSLogConfig
    IgnoreCertificateCheck bool
    LogRetention string
    LetsEncryptEnabled bool
    LetsEncryptEmail string
}

//AWSLogConfig -
type AWSLogConfig struct {
    LoggroupName string
    StreamPrefix []string
    FilterPtn string
    Profile string
    Region string
    Period string
}

//Config - Global
var Config AppConfig

//TimeISO8601LayOut
const (
    TimeISO8601LayOut = "2006-01-02T15:04:05-0700"
    AUTimeLayout = "02/01/2006 15:04:05 MST"
)

//GetDBConn -
func GetDBConn() (*sqlite3.Conn) {
    conn, err := sqlite3.Open(Config.Logdbpath)
	if err != nil {
		log.Fatalf("ERROR - can not open log database file - %v\n", err)
    }
	_dur, err := time.ParseDuration(Config.Dbtimeout)
	if err != nil {
        log.Printf("WARN - can not parse Dbtimeout string. Set default to 15 sec - %v\n", err)
        conn.BusyTimeout(15 * time.Second)
	} else{
		conn.BusyTimeout(_dur)
    }
    return conn
}

//GenerateDefaultConfig -
func GenerateDefaultConfig(opt ...interface{}) (e error) {
    defaultConfig := AppConfig {
        Port: 8000,
        Serverdomain: "localhost",
        Serverurl: "http://localhost:8000",
        IgnoreCertificateCheck: false,
        JwtKey: "ChangeThisKeyInYourSystem",
        Logdbpath: "logs.db",
        LogRetention: "90d",
        Dbtimeout: "45s",
        Commands: []Command {
            {
                Name: "example_ls",
                Path: "/bin/ls",
            },
            {
                Name: "ping",
                Path: "/bin/echo pong",
            },
        },
        Logfiles: []LogFile{
            {
                Name: "syslog",
                Paths: []string {
                    "/var/log/syslog", "/var/log/authlog", "/var/log/kern.log",
                },
                Timelayout: "Jan 02 15:04:05 2006 MST",
                Timepattern: `^([a-zA-Z]{3,3}[\s]+[\d]{0,2}[\s]+[\d]{2,2}\:[\d]{2,2}\:[\d]{2,2}) `,
                //syslog miss the year and zone. Add it in here.
                Timeadjust: "2006 MST ",
                Timestrreplace: []string{",", "."},
                Pattern: `([^\s]+) ([^\s]+) (.*)$`,
                Multilineptn: `([^\s]+.*)$`,
                Excludepatterns: []string{},
                Includepatterns: []string{},
                Appname: "",
            },
        },
        Sslcert: "",
        Sslkey: "",
        PasswordFilterPatterns: []string {`([Pp]assword|[Pp]assphrase)['"]*[\:\=]*[\s\n]*[^\s]+[\s;]`},
        LetsEncryptEnabled: false,
        LetsEncryptEmail: "someone@somewhere",
    }

    var fPath, serverurl, jwtkey, logfile, appname, sslcert, sslkey, serverdomain, logdbpath, letsencryptemail string
    var letsencryptenabled bool
    var port int

    for i, v := range(opt) {
        if i % 2 == 0 {
            key := v.(string)
            switch key {
            case "file":
                fPath = opt[i+1].(string)
            case "serverurl":
                serverurl = opt[i+1].(string)
            case "jwtkey":
                jwtkey = opt[i+1].(string)
            case "logfile":
                logfile = opt[i+1].(string)
            case "appname":
                appname = opt[i+1].(string)
            case "sslcert":
                sslcert = opt[i+1].(string)
            case "sslkey":
                sslkey = opt[i+1].(string)
            case "port":
                port = opt[i+1].(int)
            case "serverdomain":
                serverdomain = opt[i+1].(string)
            case "logdbpath":
                logdbpath = opt[i+1].(string)
            case "letsencryptenabled":
                letsencryptenabled = opt[i+1].(bool)
            case "letsencryptemail":
                letsencryptemail = opt[i+1].(string)
            }
        }
    }

    var data []byte
    if logfile != "" || serverurl != "" || jwtkey != "" || logdbpath != "" {
        var logfiles []string
        _logfiles := strings.Split(logfile, ",")
        for _, _f := range(_logfiles) {
            if _, err := os.Stat(_f); os.IsNotExist(err) {
                log.Printf("INFO - File %s does not exist. In SimpleTail mode we dont wait, Skipping\n", _f)
            } else { logfiles = append(logfiles, _f) }
        }
        _Logfiles := []LogFile{
            {
                Name: "SimpleTailLog",
                Paths: logfiles,
                Timelayout: "Jan 02 15:04:05 2006 MST",
                Timepattern: "",
                Timeadjust: "",
                Pattern: `([^\s]+.*)$`,
                Multilineptn: `^[\s]+([^\s]+.*)$`,
                Excludepatterns: []string{},
                Includepatterns: []string{},
                Appname: appname,
            },
        }

        defaultConfig.Logfiles = _Logfiles
        defaultConfig.Serverurl = serverurl
        defaultConfig.JwtKey = jwtkey
        defaultConfig.IgnoreCertificateCheck = true

        if serverdomain != "" { defaultConfig.Serverdomain = serverdomain }
        defaultConfig.Port = port
        defaultConfig.Logdbpath = logdbpath
        defaultConfig.LetsEncryptEnabled = letsencryptenabled
        defaultConfig.LetsEncryptEmail = letsencryptemail

        data, e = yaml.Marshal(defaultConfig)
        if e != nil { log.Fatalf("ERROR can not dump default config yaml")}

    } else {
        defaultConfig.Sslcert, defaultConfig.Sslkey = sslcert, sslkey
        data, e = yaml.Marshal(defaultConfig)
        if e != nil { log.Fatalf("ERROR can not dump default config yaml")}
    }
    err := ioutil.WriteFile(fPath, data, 0600)
    if err != nil {return err}
    fmt.Printf("\n\nIf you setup a server you still need to edit the config to at least supply the value of appgoogleclientid, appgoogleclientsecret, sessionkey, authorizeddomain\n\n")
    return LoadConfig(fPath)
}

// Scopes: OAuth 2.0 scopes provide a way to limit the amount of access that is granted to an access token.
var GoogleOauthConfig oauth2.Config

const OauthGoogleUrlAPI = "https://www.googleapis.com/oauth2/v2/userinfo?access_token="

//SessionStore -
var SessionStore *sessions.CookieStore

//ServerProtocol -
var ServerProtocol string

//CurrentYear - CurrentZone - Used for timeadjust
var CurrentYear, CurrentZone string

var PasswordFilterPtns []*regexp.Regexp

var JenkinsLogDataPattern *regexp.Regexp

//LoadConfig - Load all config/global regex pattern required
func LoadConfig(fPath string) (e error) {
    yamlStr, e := ioutil.ReadFile(fPath)
    if e != nil {
        return e
    }
    e = yaml.Unmarshal(yamlStr, &Config)

    _t := time.Now()
    CurrentYear = strconv.FormatInt(int64(_t.Year()), 10)
    CurrentZone, _ = _t.Zone()

    for _, ptn := range(Config.PasswordFilterPatterns){
        PasswordFilterPtns = append(PasswordFilterPtns, regexp.MustCompile(ptn))
    }

    if Config.Sslkey == "" && ! Config.LetsEncryptEnabled {
        ServerProtocol = "http"
    } else {
        ServerProtocol = "https"
    }

    GoogleOauthConfig = oauth2.Config {
        RedirectURL:  fmt.Sprintf("%s://%s:%d/auth/google/callback", ServerProtocol, Config.Serverdomain, Config.Port),
        // ClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
        ClientID:     Config.AppGoogleClientID,
        ClientSecret: Config.AppGoogleClientSecret,
        Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email"},
        Endpoint:     google.Endpoint,
    }
    SessionStore = sessions.NewCookieStore([]byte(Config.Sessionkey))
    SessionStore.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   3600 * 4,
		HttpOnly: true,
    }

    JenkinsLogDataPattern = regexp.MustCompile("\u001B\\[8mha\\:([^\u001B]+)\u001B\\[0m")
    return e
}

//GetConfig - by key and return value. Give second arg as default value.
func GetConfig(key ...string) (string) {
	DB := GetDBConn()
	defer DB.Close()
	var val string
    stmt, err := DB.Prepare("SELECT val FROM appconfig WHERE key = ?;", key[0] )
    if err != nil {
        log.Fatalf("ERROR GetConfig %v\n", err)
    }
    defer stmt.Close()
    stmt.Step()
    if err = stmt.Scan(&val); err != nil {
        log.Printf("INFO GetConfigKey %v\n", err)
    }
    if val == "" {
        log.Printf("INFO key not found\n")
		argLen := len(key)
		if argLen > 1 {
			return key[1]
		} else {
			return ""
		}
    }
	return val
}

//GetConfigSave -
func GetConfigSave(key ...string) (string) {
	v := GetConfig(key...)
	if len(key) == 2 && v == key[1] {
		SetConfig(key[0], key[1])
	}
	return v
}

//SetConfig - Set a config key with value
func SetConfig(key, val string) {
	curVal := GetConfig(key, "NOTFOUND")
	DB := GetDBConn()
	defer DB.Close()
    DB.Begin()
	if curVal != "NOTFOUND" {//Key exists, need update?
		if curVal != val {
			if e := DB.Exec(`UPDATE appconfig SET val = ? WHERE key = ?`, val, key); e != nil {
				DB.Rollback()
				log.Fatalf("ERROR %v\n", e)
			}
		}
	} else {//Not exist, just do insert
		if e := DB.Exec(`INSERT INTO appconfig(key, val) VALUES($1, $2)`, key, val); e != nil {
			DB.Rollback()
			log.Fatalf("ERROR %v\n", e)
		}
    }
    DB.Commit()
}

//DeleteConfig - delete the config key
func DeleteConfig(key string) {
	DB := GetDBConn()
	DB.Begin()
	if e := DB.Exec(`DELETE FROM appconfig WHERE key = ?`, key); e !=nil {
		DB.Rollback()
		log.Fatalf("ERROR %v\n", e)
	}
	DB.Commit()
}