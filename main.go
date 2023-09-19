package main

import (
	"context"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/firefox"
)

func main() {
	//Create context with cancel
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		log.Printf("got signal: %v", <-ch)
		cancel()
	}()

	//Set server up
	fss := http.FileServer(http.Dir("."))
	http.Handle("/", fss)
	log.Print("Listening on :8080...")
	log.Print("---> http://localhost:8080/")

	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal(err)
		}

	}()

	//Start geckodriver
	cmd := exec.Command("geckodriver")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// cmd.Stdout = os.Stdout
	err := cmd.Start()
	if err != nil {
		log.Println(err)
	}
	defer syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)

	time.Sleep(time.Second)
	caps := selenium.Capabilities{}
	caps.AddFirefox(firefox.Capabilities{
		Binary: "firefox",
		//Args:   []string{"--headless"},
	})

	//Connect to web driver
	wd, err := selenium.NewRemote(caps, "http://127.0.0.1:4444")
	if err != nil {
		log.Println(err)
	}
	defer wd.Close()

	//Navigate to http://localhost:8080/
	if err := wd.Get("http://localhost:8080/"); err != nil {
		log.Println(err)
		return
	}

	//Watch for changes
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Println(err)
	}
	defer watcher.Close()

	go func() {
		for {
			select {
			case <-watcher.Errors:
				log.Println("error event")
			case <-watcher.Events:
				log.Println("changed in ./")
				//Refresh on change
				time.Sleep(time.Millisecond * 100)
				if err := wd.Get("http://localhost:8080/"); err != nil {
					log.Println(err)
					return
				}
			Out:
				for {
					select {
					case <-watcher.Events:
						time.Sleep(time.Millisecond * 100)
					default:
						break Out
					}
				}
			}
		}
	}()

	//Add paths to watch for
	err = filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		log.Println("added: " + path)
		watcher.Add(path)
		return nil
	})
	if err != nil {
		log.Println(err)
	}

	//Wait for signal to shutdown
	for {
		select {
		case <-ctx.Done():
			return
		}
	}
}
