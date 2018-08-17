package main

import "gitlab.protectv.local/regan/flume.git"

func main() {

	flume.Configure(flume.Config{
		Development: true,
	})

	log := flume.New("root")

	log.Info("Hello World!")
	log.Info("This entry has properties", "color", "red")
	log.Debug("This is a debug message")
	log.Error("This is an error message")

}
