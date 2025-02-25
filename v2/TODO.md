- [x] merry errors don't print stacktraces when using slog's json encoder.  How values are rendered is dependent on 
  the encoder's implementation.  The TextHandler will use TextMarshaler, if implemented, then fall back to 
  `Sprintf("%+v")`.  But the JSONHandler will try JSONMarshaler, then fall back on err.Error().  We could address 
  in a couple different ways:
  - wrap error values in something that implements JSONMarshaler, and marshal merry errors to some kind of JSON
    structure
  - wrap error values in something which renders the error to a string Value with Sprintf.  this would make error 
    rendering uniform regardless of handler.  Not great, since some handlers might want to handle the native error type.
- [x] Something like hooks, but I think they can be handled more like Handler middleware
- [x] Port the console encoder
- [x] Support most of the same environment options as flume v1
- [x] experiment with replacing the weakref thing with upward pointing references (from child to parent) which just check an atomic "dirty" flag to know whether they need to reconstruct their local handlers lazily
  - Not worth it.  there is no way to do this without adding at least one additional atomic resolve to each log call
- [x] do some renaming
  - [x] Factory -> ??? maybe "Controller"?
  - [x] handlerState -> state
  - [x] delegateHandler -> delegate
- [x] still not crazy about some of the names, in particular "conf" and "delegate".  How about "sink" for the delegate handler?
- [x] add convenience methods for creating a handler *and* creating a new logger from it.
- [x] Add a convenience method for loading configuration from the environment, like in v1
- [x] Add a way to register additional handlers to "encoder" values in config, and maybe change the name "Encoder" to "Handler", "DefaultDelegate", "DefaultSink", etc
- [ ] Add an option to Config for v1 compatibility
  - installs the DetailedErrors ReplaceAttr
  - And what else?
- [x] Review ConfigFromEnv().  Not sure if I should break that down more.
- [ ] Docs
- [ ] flumetest, and could this be replaced by https://github.com/neilotoole/slogt/blob/master/slogt.go
- [ ] LoggerWriter, could this be replaced by an off the shelf sink?
- [x] Make the "logger" key name configurable
- [x] What happens when using flume with no configuration, by default?  Should it act like slog, and forward to the legacy log package?  Or should it discard, like flume v1?
      - for now, it forwards to the slog default handler, which in turn forwards to the legacy log package.  I considered adding
        a function which reverses that flow, by setting slog.SetDefault() to a flume handler...but that gets really wierd.  slog's
        package level functions, like With(), don't make much sense, since setting the slog default handler *after* calling
        slog.With(), means that child logger will be stuck with the older handler...it generally poses the same order-of-init problems
        which flume is trying to solve in general.  So, for now, I'm leaving out this type of function.  I will document that users
        *can* do this if they wish, by first setting a new default sink, then setting slog's default handler, but that there are caveats.
      - or should v2 act like v1 and not log anything until it has been configured?  Should there be an option to buffer until a configuration
        has been set, so logs won't be lost?
- [x] When using Config to set all the levels, does that clear any prior level settings?  Is there generally a way to clear/reset
      all confs?
- [x] Should there be a bridge from v1 to v2?  Add a way to direct all v1 calls to v2 calls?
  - Working on a slog handler -> zap core bridge, and a zap core -> slog handler bridge
- [x] I think the states need to be re-organized back into a parent-child graph, and sinks need to trickle down that tree.  Creating all the handlers and states in conf isn't working the way it was intended.  Rebuilding the leaf handlers is grouping the cached attrs wrong (need tests to verify this), and is also inefficient, since it creates inefficient calls to the sink's WithAttrs()
- [x] Add a middleware which supports ReplaceAttr.  Could be used to add ReplaceAttr support to Handlers which don't natively support it
  - [x] We could then promote ReplaceAttr support to the root of Config.  If the selected handler natively supports ReplaceAttr, great, otherwise we can add the middleware.  To support this, change the way handlers are registered with Config, so that each registration provides a factory method for building the handler, which can take the Config object, and adapt it to the native options that handler supports.
- [ ] should the attrs passed to Log() also be checked for LoggerKey?