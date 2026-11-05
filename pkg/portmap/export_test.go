package portmap

// SetPorts points c at test gateways listening on other ports.
func (c *Client) SetPorts(pmp uint16, ssdp string) { c.pmpPort, c.ssdpAddr = pmp, ssdp }

var ParseRoutes = parseRoutes
