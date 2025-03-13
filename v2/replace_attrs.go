package flume

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// AbbreviateLevel is a ReplaceAttr function that abbreviates log level names.
//
// It modifies the attribute if it's a log level (slog.Level) and changes the level name to its abbreviation.
// The abbreviations are:
//
//   - "DEBUG" becomes "DBG"
//   - "INFO" becomes "INF"
//   - "WARN" becomes "WRN"
//   - "ERROR" becomes "ERR"
//
// If the attribute's value is not a slog.Level, it is returned unchanged.
//
// Example:
//
//	// Create a logger with the ReplaceAttr function.
//	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//		ReplaceAttr: AbbreviateLevel,
//	}))
//
//	// Log a message.
//	logger.Debug("This is a debug message.") // Output will be: level=DBG msg="This is a debug message."
func AbbreviateLevel(_ []string, attr slog.Attr) slog.Attr {
	if attr.Value.Kind() != slog.KindAny {
		return attr
	}

	if lvl, ok := attr.Value.Any().(slog.Level); ok {
		strLvl := lvl.String()
		switch strLvl[0] {
		case 'D':
			strLvl = dbgAbbrev + strLvl[5:]
		case 'W':
			strLvl = wrnAbbrev + strLvl[4:]
		case 'E':
			strLvl = errAbbrev + strLvl[5:]
		case 'I':
			strLvl = infAbbrev + strLvl[4:]
		}
		attr.Value = slog.StringValue(strLvl)
	}

	return attr
}

// FormatTimes returns a ReplaceAttr function that formats time values according to the specified format.
//
// It modifies an attribute if its value is a time.Time. The time is formatted using the provided format string,
// and the attribute's value is updated to the formatted time string.
//
// If the attribute's value is not a time.Time, it is returned unchanged.
//
// Args:
//
//	format string: The format string to use for formatting time values (e.g., time.DateTime, "2006-01-02", "15:04:05").
//
// Returns:
//
//	func([]string, slog.Attr) slog.Attr: A ReplaceAttr function that formats time values.
//
// Example:
//
//		// Create a ReplaceAttr function that formats times using a custom format.
//		customTimeFormat := FormatTimes("2006-01-02 15:04:05")
//
//		// Create a logger with the ReplaceAttr function.
//		logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//		    ReplaceAttr: customTimeFormat,
//		}))
//
//		// Log a message with a time attribute.
//		logger.Info("Time example", slog.Time("now", time.Now()))
//		// Output might be: level=INFO msg="Time example" now="2023-10-27 10:30:00"
//
//	 // Create a ReplaceAttr function that formats times using a predefined format.
//	 dateTimeFormat := FormatTimes(time.DateTime)
//
//	 // Create a logger with the ReplaceAttr function.
//	 logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//	     ReplaceAttr: dateTimeFormat,
//	 }))
//
//	 // Log a message with a time attribute.
//	 logger.Info("Time example", slog.Time("now", time.Now()))
//	 // Output might be: level=INFO msg="Time example" now="2023-10-27 10:30:00.000"
func FormatTimes(format string) func([]string, slog.Attr) slog.Attr {
	return transformValuesOfKind(slog.KindTime, func(v slog.Value) slog.Value {
		return slog.StringValue(v.Time().Format(format))
	})
}

// SimpleTime returns a ReplaceAttr function that formats time values to a simple time format: "15:04:05.000".
//
// It's a convenience function that uses FormatTimes internally with a predefined format.
//
// Example:
//
//	// Create a logger with the ReplaceAttr function.
//	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
//	    ReplaceAttr: SimpleTime(),
//	}))
//
//	// Log a message with a time attribute.
//	logger.Info("Time example", slog.Time("now", time.Now()))
//	// Output might be: level=INFO msg="Time example" now="10:30:00.000"
func SimpleTime() func([]string, slog.Attr) slog.Attr {
	return FormatTimes("15:04:05.000")
}

// ISO8601Time returns a ReplaceAttr function that formats time to the ISO8601
// compliant format used by flumev1 and zap.  It is here to provide backward
// compatibility with flume v1, but should probably be avoided for new
// applications, in favor of the default formatting selected by the handler.
// The json and text handlers in the slog package use RFC3339, which is
// mostly compatible with ISO8601 anyway (see https://ijmacd.github.io/rfc3339-iso8601/
// for how they overlap).
func ISO8601Time() func([]string, slog.Attr) slog.Attr {
	return FormatTimes("2006-01-02T15:04:05.000Z0700")
}

// RFC3339MillisTime returns a ReplaceAttr function which formats time to the
// format that slog's text handler uses.
func RFC3339MillisTime() func(_ []string, a slog.Attr) slog.Attr {
	return transformValuesOfKind(slog.KindTime, func(v slog.Value) slog.Value {
		t := v.Time()
		// Format according to time.RFC3339Nano since it is highly optimized,
		// but truncate it to use millisecond resolution.
		// Unfortunately, that format trims trailing 0s, so add 1/10 millisecond
		// to guarantee that there are exactly 4 digits after the period.
		const prefixLen = len("2006-01-02T15:04:05.000")
		t = t.Truncate(time.Millisecond).Add(time.Millisecond / 10)

		// len(time.RFC3339Nano), but allocating on stack
		ba := [35]byte{}
		b := ba[:0]
		b = t.AppendFormat(b, time.RFC3339Nano)
		b = append(b[:prefixLen], b[prefixLen+1:]...) // drop the 4th digit

		return slog.StringValue(string(b))
	})
}

// FixedTime replaces all time attributes with a fixed value.  Useful in
// testing to get a predictable log line.
func FixedTime(t time.Time) func(_ []string, a slog.Attr) slog.Attr {
	return transformValuesOfKind(slog.KindTime, func(_ slog.Value) slog.Value {
		return slog.TimeValue(t)
	})
}

// SecondsDuration formats durations as fractional seconds.  This may be used
// to mimic the default encoding of durations in flumev1/zap.
func SecondsDuration() func(_ []string, a slog.Attr) slog.Attr {
	return transformValuesOfKind(slog.KindDuration, func(v slog.Value) slog.Value {
		return slog.Float64Value(v.Duration().Seconds())
	})
}

func transformValuesOfKind(kind slog.Kind, transform func(slog.Value) slog.Value) func([]string, slog.Attr) slog.Attr {
	return func(_ []string, a slog.Attr) slog.Attr {
		if a.Value.Kind() == kind {
			a.Value = transform(a.Value)
		}
		return a
	}
}

type detailedJSONError struct {
	error
	fmt.Formatter
}

func (e detailedJSONError) MarshalJSON() ([]byte, error) {
	return json.Marshal(fmt.Sprintf("%+v", e.error)) //nolint:wrapcheck
}

// DetailedErrors is a ReplaceAttr function which ensures that errors
// implement fmt.Formatter are rendered the same by the JSONHandler as
// by the TextHandler.  By default, the slog.JSONHandler renders errors
// as err.Error(), where as the slog.TextHandler will render it with
// fmt.Sprintf("%+v", err).
//
// When using DetailedErrors, if any error values implement fmt.Formatter (
// and therefore *may* implement some form of detailed error printing), and
// *do not* already implement json.Marshaler, then the value is wrapped in
// an error implementation which implements json.Marshaler, and marshals
// the error to a string using fmt.Sprintf("%+v", err).
func DetailedErrors(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() != slog.KindAny {
		return a
	}

	if err, ok := a.Value.Any().(error); ok {
		if formatter, ok := err.(fmt.Formatter); ok {
			if _, ok = err.(json.Marshaler); !ok {
				a.Value = slog.AnyValue(detailedJSONError{err, formatter})
			}
		}
	}

	return a
}

func applyReplaceAttrs(groups []string, a slog.Attr, fns []func([]string, slog.Attr) slog.Attr) slog.Attr {
	// assume that the value has already been resolved by the calling handler.  That's part
	// of the contract
	for _, fn := range fns {
		if fn == nil {
			continue
		}
		a = fn(groups, a)

		if a.Equal(slog.Attr{}) {
			// one of the functions returned an empty attr, which
			// is the equivalent of deleting the attr, so we're done.
			return a
		}

		a.Value = a.Value.Resolve()
		if a.Value.Kind() == slog.KindGroup {
			// ReplaceAttr is only run on non-group attrs.  We can stop.
			// The handler will take care of calling us again on the members.
			return a
		}
	}

	return a
}

// ChainReplaceAttrs composes a series of ReplaceAttr functions into a single ReplaceAttr function.
func ChainReplaceAttrs(fns ...func([]string, slog.Attr) slog.Attr) func([]string, slog.Attr) slog.Attr {
	switch len(fns) {
	case 0:
		// no op, return nil func
		return nil
	case 1:
		return fns[0]
	default:
		return func(groups []string, a slog.Attr) slog.Attr {
			return applyReplaceAttrs(groups, a, fns)
		}
	}
}
