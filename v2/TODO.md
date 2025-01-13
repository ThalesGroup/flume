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
- [ ] still not crazy about some of the names, in particular "conf" and "delegate".  How about "sink" for the delegate handler?
- [x] add convenience methods for creating a handler *and* creating a new logger from it.
- [x] Add a convenience method for loading configuration from the environment, like in v1
- [ ] Add a way to register additional handlers to "encoder" values in config, and maybe change the name "Encoder" to "Handler", "DefaultDelegate", "DefaultSink", etc
- [ ] Add an option to Config for v1 compatibility
  - installs the DetailedErrors ReplaceAttr
  - And what else?