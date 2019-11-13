package main

import (
	"github.com/hpcloud/tail"
	"sync"
	"os"
	"fmt"
	"log"
	"flag"
	"./cmd"
)

func main() {
	defaultConfig :=  fmt.Sprintf("%s/.nsca-go.yaml", os.Getenv("HOME"))
	configFile := flag.String("c", defaultConfig, fmt.Sprintf("Config file, default %s", defaultConfig))
	mode := flag.String("m", "client", "run mode. Can be server|client|tail|reset.\nserver - start nsca server and wait for command.\nclient - take another option -cmd which is the command to send to the server.\ntail - tail the log and send to the log server.\nreset - reset the config using default")
	cmdName := flag.String("cmd", "", "Command name")
	tailFollow := flag.Bool("tailf", false, "Tail mode follow|")
	flag.Parse()

	tailCfg := tail.Config{
		// Location:    seek,
		ReOpen:      *tailFollow,
		MustExist:   false,
		Poll:        false,
		Pipe:        false,
		Follow:      *tailFollow,
		MaxLineSize: 0,
	}

	e := cmd.LoadConfig(*configFile)
	if e != nil {
		log.Printf("Error reading config file. %v\nGenerating new one\n", e)
		if e = cmd.GenerateDefaultConfig(*configFile); e != nil {
			log.Fatalf("ERROR can not geenrate config file %v\n", e)
		}
	}
	switch *mode {
	case "server":
		cmd.StartServer()
	case "client":
		cmd.RunCommand(*cmdName)
	case "tail":
		var wg sync.WaitGroup
		for _, _logFile := range(cmd.Config.Logfiles) {
			_tailLogConfig := cmd.TailLogConfig{
				LogFile: _logFile,
				TailConfig: tailCfg,
			}
			log.Printf("Spawn tailling process ...\n")
			wg.Add(1)
			go cmd.TailLog(&_tailLogConfig, &wg)
		}
		wg.Wait()
	case "reset":
		cmd.GenerateDefaultConfig(*configFile)
	}
}