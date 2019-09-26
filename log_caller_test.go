package flume

func logSomething(l Logger) {
	l.Info("something")
}

// DON'T EDIT THIS FILE
// the tests which verify that we're capturing the caller correctly rely on
// the logSomething() function not moving somewhere else in the file.  The log
// line must be on line 4.  If you need to edit this file, be sure to update the
// logSomethingLine variable below to the new line number of the log call.
//
// If you rename or move this file, update the logSomethingFile variable.

var logSomethingLine = 4
var logSomethingFile = "flume/log_caller_test.go"
