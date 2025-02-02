package flume

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewController(t *testing.T) {
	ctl := NewController(noop)

	assert.Equal(t, slog.LevelInfo, ctl.DefaultLevel())
	assert.Equal(t, noop, ctl.DefaultSink())
}

func TestController_Logger(t *testing.T) {
	ctl := NewController(noop)
	buf := bytes.NewBuffer(nil)
	ctl.SetSink("blue", slog.NewTextHandler(buf, &slog.HandlerOptions{
		ReplaceAttr: removeKeys(slog.TimeKey),
	}))

	l := ctl.Logger("blue")
	require.NotNil(t, l)

	l.Info("hi")
	assert.Equal(t, "level=INFO msg=hi logger=blue\n", buf.String())
}

func TestController_Handler(t *testing.T) {
	ctl := NewController(noop)
	buf := bytes.NewBuffer(nil)
	ctl.SetSink("blue", slog.NewTextHandler(buf, nil))

	h := ctl.Handler("blue")
	err := h.Handle(context.Background(), slog.NewRecord(time.Time{}, slog.LevelInfo, "hi", 0))
	require.NoError(t, err)
	assert.Equal(t, "level=INFO msg=hi logger=blue\n", buf.String())
}

func TestController_Sinks(t *testing.T) {
	buf := bytes.NewBuffer(nil)
	redSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "red")})
	blueSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "blue")})
	yellowSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "yellow")})
	defSink := slog.NewTextHandler(buf, nil).WithAttrs([]slog.Attr{slog.String("sink", "def")})

	testCases := []struct {
		desc      string
		ctlFn     func() *Controller
		want      map[string]string
		wantSinks map[string]slog.Handler
	}{
		{
			desc: "set sinks",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", blueSink)
				ctl.SetSink("red", redSink)
				return ctl
			},
			want: map[string]string{
				"blue":   "sink=blue",
				"red":    "sink=red",
				"yellow": "sink=def",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   blueSink,
				"red":    redSink,
				"yellow": defSink,
				"*":      defSink,
			},
		},
		{
			desc: "set nil sink sets a noop sink",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", nil)
				return ctl
			},
			want: map[string]string{
				"blue":   "",
				"yellow": "sink=def",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   noop,
				"yellow": defSink,
				"*":      defSink,
			},
		},
		{
			desc: "SetSink(*) sets the default sink",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", blueSink)
				ctl.SetSink("*", redSink)
				return ctl
			},
			want: map[string]string{
				"blue":   "sink=blue",
				"yellow": "sink=red",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   blueSink,
				"yellow": redSink,
				"*":      redSink,
			},
		},
		{
			desc: "SetDefaultSink sets the default sink",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", blueSink)
				ctl.SetDefaultSink(redSink)
				return ctl
			},
			want: map[string]string{
				"blue":   "sink=blue",
				"yellow": "sink=red",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   blueSink,
				"yellow": redSink,
				"*":      redSink,
			},
		},
		{
			desc: "SetSinks",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSinks(map[string]slog.Handler{
					"blue": blueSink,
					"red":  redSink,
					"*":    yellowSink,
				}, true)
				return ctl
			},
			want: map[string]string{
				"blue":   "sink=blue",
				"red":    "sink=red",
				"yellow": "sink=yellow",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   blueSink,
				"red":    redSink,
				"yellow": yellowSink,
				"*":      yellowSink,
			},
		},
		{
			desc: "SetSinks with replace",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", yellowSink)
				ctl.SetSink("red", redSink)
				ctl.SetSinks(map[string]slog.Handler{
					"blue": blueSink,
				}, true)
				return ctl
			},
			want: map[string]string{
				"blue": "sink=blue",
				"red":  "sink=def", // red should have been reset because replace=true
			},
			wantSinks: map[string]slog.Handler{
				"blue": blueSink,
				"red":  defSink,
			},
		},
		{
			desc: "SetSinks with replace=false appends",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", yellowSink)
				ctl.SetSink("red", redSink)
				ctl.SetSinks(map[string]slog.Handler{
					"blue": blueSink,
				}, false)
				return ctl
			},
			want: map[string]string{
				"blue": "sink=blue",
				"red":  "sink=red",
			},
			wantSinks: map[string]slog.Handler{
				"blue": blueSink,
				"red":  redSink,
				"*":    defSink,
			},
		},
		{
			desc: "ClearSinks resets all sinks to default",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", blueSink)
				ctl.SetSink("red", redSink)
				ctl.SetSink("yellow", yellowSink)
				ctl.ClearSinks()
				return ctl
			},
			want: map[string]string{
				"blue":   "sink=def",
				"red":    "sink=def",
				"yellow": "sink=def",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   defSink,
				"red":    defSink,
				"yellow": defSink,
				"*":      defSink,
			},
		},
		{
			desc: "ClearSink resets single sink to default",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", blueSink)
				ctl.SetSink("red", redSink)
				ctl.ClearSink("blue")
				return ctl
			},
			want: map[string]string{
				"blue": "sink=def",
				"red":  "sink=red",
			},
			wantSinks: map[string]slog.Handler{
				"blue": defSink,
				"red":  redSink,
				"*":    defSink,
			},
		},
		{
			desc: "ClearSink with * does nothing",
			ctlFn: func() *Controller {
				ctl := NewController(defSink)
				ctl.SetSink("blue", blueSink)
				ctl.ClearSink("*")
				return ctl
			},
			want: map[string]string{
				"blue":   "sink=blue",
				"yellow": "sink=def",
			},
			wantSinks: map[string]slog.Handler{
				"blue":   blueSink,
				"yellow": defSink,
				"*":      defSink,
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctl := tC.ctlFn()
			for logger, want := range tC.want {
				buf.Reset()
				ctl.Logger(logger).Info("hi")
				if want == "" {
					assert.Empty(t, buf.String(), "logger: %s", logger)
				} else {
					assert.Contains(t, buf.String(), want, "logger: %s", logger)
				}
			}
			for logger, want := range tC.wantSinks {
				assert.Equal(t, want, ctl.Sink(logger), "logger: %s", logger)
				if logger == "*" {
					assert.Equal(t, want, ctl.DefaultSink())
				}
			}
		})
	}
}

func TestController_Levels(t *testing.T) {
	testCases := []struct {
		desc     string
		want     map[string]slog.Level
		configFn func(*Controller)
	}{
		{
			desc: "SetLevel",
			configFn: func(c *Controller) {
				c.SetLevel("blue", slog.LevelWarn)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelWarn,
				"red":  slog.LevelInfo,
				"*":    slog.LevelInfo,
			},
		},
		{
			desc: "SetLevel(*) sets default level",
			configFn: func(c *Controller) {
				c.SetLevel("*", slog.LevelWarn)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelWarn,
				"*":    slog.LevelWarn,
			},
		},
		{
			desc: "ClearLevels",
			configFn: func(c *Controller) {
				c.SetDefaultLevel(slog.LevelDebug)
				c.SetLevel("blue", slog.LevelWarn)
				c.SetLevel("red", slog.LevelError)
				c.ClearLevels()
			},
			want: map[string]slog.Level{
				"blue": slog.LevelDebug,
				"red":  slog.LevelDebug,
				"*":    slog.LevelDebug,
			},
		},
		{
			desc: "SetLevels",
			configFn: func(c *Controller) {
				c.SetLevels(map[string]slog.Level{
					"blue": slog.LevelInfo,
					"red":  slog.LevelDebug,
					"*":    slog.LevelWarn,
				}, false)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelInfo,
				"red":  slog.LevelDebug,
				"*":    slog.LevelWarn,
			},
		},
		{
			desc: "SetLevels with replace",
			configFn: func(c *Controller) {
				c.SetLevel("blue", slog.LevelWarn)
				c.SetLevel("red", slog.LevelDebug)
				c.SetLevel("*", slog.LevelError)
				c.SetLevels(map[string]slog.Level{
					"blue": slog.LevelError,
					"*":    slog.LevelWarn,
				}, true)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelError,
				"red":  slog.LevelWarn,
				"*":    slog.LevelWarn,
			},
		},
		{
			desc: "SetLevels with replace won't clear default level",
			configFn: func(c *Controller) {
				c.SetLevel("blue", slog.LevelWarn)
				c.SetLevel("red", slog.LevelDebug)
				c.SetLevel("*", slog.LevelError)
				c.SetLevels(map[string]slog.Level{
					"blue": slog.LevelDebug,
				}, true)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelDebug,
				"red":  slog.LevelError,
				"*":    slog.LevelError,
			},
		},
		{
			desc: "SetLevels with !replace appends",
			configFn: func(c *Controller) {
				c.SetLevel("blue", slog.LevelWarn)
				c.SetLevel("red", slog.LevelDebug)
				c.SetLevel("*", slog.LevelError)
				c.SetLevels(map[string]slog.Level{
					"blue": slog.LevelDebug,
					"*":    slog.LevelWarn,
				}, false)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelDebug,
				"red":  slog.LevelDebug,
				"*":    slog.LevelWarn,
			},
		},
		{
			desc: "DefaultLevel",
			configFn: func(c *Controller) {
				c.SetDefaultLevel(slog.LevelDebug)
			},
			want: map[string]slog.Level{
				"blue": slog.LevelDebug,
				"red":  slog.LevelDebug,
				"*":    slog.LevelDebug,
			},
		},
		{
			desc: "ClearLevel resets single level to default",
			configFn: func(c *Controller) {
				c.SetDefaultLevel(slog.LevelDebug)
				c.SetLevel("blue", slog.LevelWarn)
				c.SetLevel("red", slog.LevelError)
				c.ClearLevel("blue")
			},
			want: map[string]slog.Level{
				"blue": slog.LevelDebug,
				"red":  slog.LevelError,
				"*":    slog.LevelDebug,
			},
		},
		{
			desc: "ClearLevel with * does nothing",
			configFn: func(c *Controller) {
				c.SetDefaultLevel(slog.LevelDebug)
				c.SetLevel("blue", slog.LevelWarn)
				c.ClearLevel("*")
			},
			want: map[string]slog.Level{
				"blue": slog.LevelWarn,
				"*":    slog.LevelDebug,
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			ctl := NewController(noop)
			tC.configFn(ctl)
			for name, want := range tC.want {
				assert.Equal(t, want, ctl.Level(name))
				if name == allHandlers {
					assert.Equal(t, want, ctl.DefaultLevel())
				}
			}
		})
	}
}

func TestController_Middleware(t *testing.T) {
	tests := []struct {
		name     string
		configFn func(*Controller)
		wantRed  string
		wantBlue string
	}{
		{
			name:     "Use with name",
			wantRed:  "level=INFO msg=hi logger=red\n",
			wantBlue: "level=INFO msg=hi logger=blue size=big\n",
			configFn: func(c *Controller) {
				c.Use("blue", addAttrMW(slog.String("size", "big")))
			},
		},
		{
			name:     "Use with *",
			wantRed:  "level=INFO msg=hi logger=red size=big\n",
			wantBlue: "level=INFO msg=hi logger=blue size=big\n",
			configFn: func(c *Controller) {
				c.Use("*", addAttrMW(slog.String("size", "big")))
			},
		},
		{
			name:     "UseDefault",
			wantRed:  "level=INFO msg=hi logger=red size=big\n",
			wantBlue: "level=INFO msg=hi logger=blue size=big\n",
			configFn: func(c *Controller) {
				c.UseDefault(addAttrMW(slog.String("size", "big")))
			},
		},
		{
			name:     "global and local middleware",
			wantRed:  "level=INFO msg=hi logger=red size=big\n",
			wantBlue: "level=INFO msg=hi logger=blue size=big flavor=vanilla\n",
			configFn: func(c *Controller) {
				c.Use("*", addAttrMW(slog.String("size", "big")))
				c.Use("blue", addAttrMW(slog.String("flavor", "vanilla")))
			},
		},
		{
			name:     "ClearMiddleware",
			wantRed:  "level=INFO msg=hi logger=red\n",
			wantBlue: "level=INFO msg=hi logger=blue\n",
			configFn: func(c *Controller) {
				c.Use("*", addAttrMW(slog.String("size", "big")))
				c.Use("blue", addAttrMW(slog.String("flavor", "vanilla")))
				c.ClearMiddleware()
			},
		},
		{
			name:     "SetMiddleware",
			wantRed:  "level=INFO msg=hi logger=red size=big\n",
			wantBlue: "level=INFO msg=hi logger=blue size=big flavor=vanilla\n",
			configFn: func(c *Controller) {
				c.SetMiddleware(map[string][]Middleware{
					"blue": {addAttrMW(slog.String("flavor", "vanilla"))},
					"*":    {addAttrMW(slog.String("size", "big"))},
				}, false)
			},
		},
		{
			name:     "SetMiddleware with replace",
			wantRed:  "level=INFO msg=hi logger=red size=small\n",
			wantBlue: "level=INFO msg=hi logger=blue size=small flavor=strawberry\n",
			configFn: func(c *Controller) {
				c.Use("blue", addAttrMW(slog.String("flavor", "vanilla")))
				c.Use("red", addAttrMW(slog.String("weight", "heavy")))
				c.UseDefault(addAttrMW(slog.String("size", "big")))
				c.SetMiddleware(map[string][]Middleware{
					"blue": {addAttrMW(slog.String("flavor", "strawberry"))},
					"*":    {addAttrMW(slog.String("size", "small"))},
				}, true)
			},
		},
		{
			name:     "SetMiddleware with replace will clear the global middleware",
			wantRed:  "level=INFO msg=hi logger=red\n",
			wantBlue: "level=INFO msg=hi logger=blue flavor=strawberry\n",
			configFn: func(c *Controller) {
				c.Use("blue", addAttrMW(slog.String("flavor", "vanilla")))
				c.UseDefault(addAttrMW(slog.String("size", "big")))
				c.SetMiddleware(map[string][]Middleware{
					"blue": {addAttrMW(slog.String("flavor", "strawberry"))},
				}, true)
			},
		},
		{
			name:     "SetMiddleware with replace=false should append",
			wantRed:  "level=INFO msg=hi logger=red size=big size=small weight=heavy\n",
			wantBlue: "level=INFO msg=hi logger=blue size=big size=small flavor=vanilla temper=calm\n",
			configFn: func(c *Controller) {
				c.Use("blue", addAttrMW(slog.String("flavor", "vanilla")))
				c.Use("red", addAttrMW(slog.String("weight", "heavy")))
				c.UseDefault(addAttrMW(slog.String("size", "big")))
				c.SetMiddleware(map[string][]Middleware{
					"blue": {addAttrMW(slog.String("temper", "calm"))},
					"*":    {addAttrMW(slog.String("size", "small"))},
				}, false)
			},
		},
	}

	for _, tC := range tests {
		t.Run(tC.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			opts := slog.HandlerOptions{ReplaceAttr: removeKeys(slog.TimeKey)}
			ctl := NewController(slog.NewTextHandler(buf, &opts))
			tC.configFn(ctl)

			ctl.Logger("blue").Info("hi")
			assert.Equal(t, tC.wantBlue, buf.String())
			buf.Reset()
			ctl.Logger("red").Info("hi")
			assert.Equal(t, tC.wantRed, buf.String())
		})
	}
}

func addAttrMW(attr slog.Attr) Middleware {
	return HandlerMiddlewareFunc(func(ctx context.Context, record slog.Record, next slog.Handler) error {
		record.AddAttrs(attr)
		return next.Handle(ctx, record)
	})
}
