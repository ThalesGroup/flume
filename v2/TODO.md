- [ ] merry errors don't print stacktraces when using slog's json encoder.  How values are rendered is dependent on 
  the encoder's implementation.  The TextHandler will use TextMarshaler, if implemented, then fall back to 
  `Sprintf("%+v")`.  But the JSONHandler will try JSONMarshaler, then fall back on err.Error().  We could address 
  in a couple different ways:
  - wrap error values in something that implements JSONMarshaler, and marshal merry errors to some kind of JSON
    structure
  - wrap error values in something which renders the error to a string Value with Sprintf.  this would make error 
    rendering uniform regardless of handler.  Not great, since some handlers 
- [ ] Something like hooks, but I think they can be handled more like Handler middleware
- [ ] Pick a better name for Factory.  Maybe Controller?
- [ ] Port the console encoder
- [ ] Support most of the same environment options as flume v1
- [ ] 