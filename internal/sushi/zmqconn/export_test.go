package zmqconn

// Lookup exposes the wire ids of a method ("/sushi_rpc.Svc/Method") to the external tests.
func Lookup(method string) (service, endpoint int32, err error) {
	ep, err := lookup(method)
	return ep.service, ep.endpoint, err
}

// MessageCode exposes the notification code of a top-level message name.
func MessageCode(name string) (int32, bool) {
	c, ok := msgCodes[protoName(name)]
	return c, ok
}
