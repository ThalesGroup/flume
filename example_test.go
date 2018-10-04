package flume_test

import "github.com/gemalto/flume"

func Example() {

	flume.Configure(flume.Config{
		Development:  true,
		DefaultLevel: flume.DebugLevel,
		Encoding:     "ltsv",
	})

	log := flume.New("root")

	log.Info("Hello World!")
	log.Info("This entry has properties", "color", "red")
	log.Debug("This is a debug message")
	log.Error("This is an error message")
	log.Info("This message has a multiline value", "essay", `Four score and seven years ago
our fathers brought forth on this continent, a new nation, 
conceived in Liberty, and dedicated to the proposition that all men are created equal.`)

}
