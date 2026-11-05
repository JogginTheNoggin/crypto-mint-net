package portmap

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const ssdpAddr = "239.255.255.250:1900"

// The gateway device types searched for, and the services a mapping is
// made through.
var (
	igdTypes     = []string{"urn:schemas-upnp-org:device:InternetGatewayDevice:2", "urn:schemas-upnp-org:device:InternetGatewayDevice:1"}
	wanServices  = []string{"urn:schemas-upnp-org:service:WANIPConnection:", "urn:schemas-upnp-org:service:WANPPPConnection:"}
	upnpDescName = "poo node"
)

// onGateway checks that raw is a plain http URL whose host is gw's own IP
// address, and returns it parsed. A host name is refused: it could lead
// anywhere.
func onGateway(raw string, gw netip.Addr) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" {
		return nil, fmt.Errorf("URL %q is not plain http", raw)
	}
	a, err := netip.ParseAddr(u.Hostname())
	if err != nil || a.Unmap() != gw.Unmap() {
		return nil, fmt.Errorf("URL %q is not on the gateway %s", raw, gw)
	}
	if u.User != nil {
		return nil, fmt.Errorf("URL %q carries a user", raw)
	}
	return u, nil
}

// httpClient takes no proxy and follows no redirect.
func (c *Client) httpClient() *http.Client {
	return &http.Client{
		Timeout:       c.timeout(),
		Transport:     &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: c.timeout()}).DialContext, MaxResponseHeaderBytes: 16 << 10, DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// readCapped reads at most MaxReply bytes and fails on more.
func readCapped(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxReply+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxReply {
		return nil, fmt.Errorf("reply over %d bytes", MaxReply)
	}
	return b, nil
}

// discover finds the gateway's description URL by SSDP. Only answers from
// the gateway's address naming a URL on it are taken.
func (c *Client) discover(ctx context.Context, gw netip.Addr) (*url.URL, error) {
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	dst := ssdpAddr
	if c.ssdpAddr != "" {
		dst = c.ssdpAddr
	}
	to, err := net.ResolveUDPAddr("udp4", dst)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(c.timeout())
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	stop := context.AfterFunc(ctx, func() { conn.SetReadDeadline(time.Now()) })
	defer stop()
	for _, st := range igdTypes {
		msg := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 2\r\nST: " + st + "\r\n\r\n"
		if _, err := conn.WriteToUDP([]byte(msg), to); err != nil {
			return nil, err
		}
	}
	conn.SetReadDeadline(deadline)
	buf := make([]byte, 2048)
	var last error = errors.New("no gateway answered the search")
	for {
		n, from, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, last
		}
		if from.Addr().Unmap() != gw.Unmap() {
			continue
		}
		loc := ssdpLocation(buf[:n])
		if loc == "" {
			continue
		}
		u, err := onGateway(loc, gw)
		if err != nil {
			last = err
			continue
		}
		return u, nil
	}
}

// ssdpLocation returns the LOCATION header of an SSDP answer.
func ssdpLocation(b []byte) string {
	lines := strings.Split(string(b), "\r\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "HTTP/1.1 200") {
		return ""
	}
	for _, l := range lines[1:] {
		k, v, ok := strings.Cut(l, ":")
		if ok && strings.EqualFold(strings.TrimSpace(k), "location") {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

type xService struct {
	ServiceType string `xml:"serviceType"`
	ControlURL  string `xml:"controlURL"`
}

type xDevice struct {
	Services []xService `xml:"serviceList>service"`
	Devices  []xDevice  `xml:"deviceList>device"`
}

type xRoot struct {
	URLBase string  `xml:"URLBase"`
	Device  xDevice `xml:"device"`
}

func (d xDevice) find(depth int) (xService, bool) {
	if depth > 8 {
		return xService{}, false
	}
	for _, s := range d.Services {
		for _, w := range wanServices {
			if strings.HasPrefix(s.ServiceType, w) && s.ControlURL != "" {
				return s, true
			}
		}
	}
	for _, sub := range d.Devices {
		if s, ok := sub.find(depth + 1); ok {
			return s, true
		}
	}
	return xService{}, false
}

// control finds the WAN connection service of the description at loc.
func (c *Client) control(ctx context.Context, gw netip.Addr, loc *url.URL) (ctl, service string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, loc.String(), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("description: %s", resp.Status)
	}
	body, err := readCapped(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("description: %w", err)
	}
	var root xRoot
	if err := xml.Unmarshal(body, &root); err != nil {
		return "", "", fmt.Errorf("description: %w", err)
	}
	s, ok := root.Device.find(0)
	if !ok {
		return "", "", errors.New("description names no WAN connection service")
	}
	for _, r := range s.ServiceType {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ':' || r == '-' || r == '.') {
			return "", "", fmt.Errorf("service type %q has odd characters", s.ServiceType)
		}
	}
	base := loc
	if root.URLBase != "" {
		if base, err = onGateway(root.URLBase, gw); err != nil {
			return "", "", err
		}
	}
	ref, err := url.Parse(s.ControlURL)
	if err != nil {
		return "", "", err
	}
	u, err := onGateway(base.ResolveReference(ref).String(), gw)
	if err != nil {
		return "", "", err
	}
	return u.String(), s.ServiceType, nil
}

// upnpError is a SOAP fault's UPnP error code.
type upnpError struct {
	Code int
	Desc string
}

func (e *upnpError) Error() string { return fmt.Sprintf("UPnP error %d %s", e.Code, e.Desc) }

// soap calls action on the service at ctl and returns the reply's
// elements by local name.
func (c *Client) soap(ctx context.Context, ctl, service, action string, args [][2]string) (map[string]string, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	fmt.Fprintf(&b, `<u:%s xmlns:u="%s">`, action, service)
	for _, a := range args {
		fmt.Fprintf(&b, "<%s>", a[0])
		xml.EscapeText(&b, []byte(a[1]))
		fmt.Fprintf(&b, "</%s>", a[0])
	}
	fmt.Fprintf(&b, `</u:%s></s:Body></s:Envelope>`, action)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ctl, &b)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPAction", `"`+service+"#"+action+`"`)
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readCapped(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", action, err)
	}
	vals := soapValues(body)
	if resp.StatusCode != http.StatusOK {
		if code, err := strconv.Atoi(vals["errorCode"]); err == nil {
			return nil, &upnpError{code, vals["errorDescription"]}
		}
		return nil, fmt.Errorf("%s: %s", action, resp.Status)
	}
	return vals, nil
}

// soapValues returns the text of every leaf element by local name.
func soapValues(body []byte) map[string]string {
	out := map[string]string{}
	d := xml.NewDecoder(bytes.NewReader(body))
	var name string
	var text []byte
	for {
		tok, err := d.Token()
		if err != nil {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name, text = t.Name.Local, nil
		case xml.CharData:
			if len(text) < 1024 {
				text = append(text, t...)
			}
		case xml.EndElement:
			if t.Name.Local == name {
				out[name] = strings.TrimSpace(string(text))
			}
			name = ""
		}
	}
}

func (c *Client) localTo(gw netip.Addr) (netip.Addr, error) {
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(netip.AddrPortFrom(gw, 1900)))
	if err != nil {
		return netip.Addr{}, err
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).AddrPort().Addr().Unmap(), nil
}

func (c *Client) upnpMap(ctx context.Context, gw netip.Addr, internal uint16, lifetime time.Duration, prev *Mapping) (Mapping, error) {
	var ctl, service string
	ext := internal
	if prev != nil {
		ctl, service, ext = prev.control, prev.service, prev.External.Port()
	} else {
		loc, err := c.discover(ctx, gw)
		if err != nil {
			return Mapping{}, err
		}
		if ctl, service, err = c.control(ctx, gw, loc); err != nil {
			return Mapping{}, err
		}
	}
	local, err := c.localTo(gw)
	if err != nil {
		return Mapping{}, err
	}
	add := func(life uint32) error {
		_, err := c.soap(ctx, ctl, service, "AddPortMapping", [][2]string{
			{"NewRemoteHost", ""}, {"NewExternalPort", strconv.Itoa(int(ext))}, {"NewProtocol", "TCP"},
			{"NewInternalPort", strconv.Itoa(int(internal))}, {"NewInternalClient", local.String()},
			{"NewEnabled", "1"}, {"NewPortMappingDescription", upnpDescName}, {"NewLeaseDuration", strconv.FormatUint(uint64(life), 10)},
		})
		return err
	}
	life := lifetimeSecs(lifetime)
	err = add(life)
	var ue *upnpError
	if errors.As(err, &ue) && ue.Code == 725 {
		// OnlyPermanentLeasesSupported: the mapping lasts until removed,
		// which a clean stop does.
		life = 0
		err = add(0)
	}
	if err != nil {
		return Mapping{}, err
	}
	vals, err := c.soap(ctx, ctl, service, "GetExternalIPAddress", nil)
	if err != nil {
		return Mapping{}, err
	}
	ip, err := netip.ParseAddr(vals["NewExternalIPAddress"])
	if err != nil || !ip.Unmap().Is4() {
		return Mapping{}, fmt.Errorf("external address %q is not an IPv4 address", vals["NewExternalIPAddress"])
	}
	return Mapping{Method: MethodUPnP, External: netip.AddrPortFrom(ip.Unmap(), ext), Internal: internal,
		Lifetime: time.Duration(life) * time.Second, control: ctl, service: service}, nil
}

func (c *Client) upnpUnmap(ctx context.Context, gw netip.Addr, m Mapping) error {
	if _, err := onGateway(m.control, gw); err != nil {
		return err
	}
	_, err := c.soap(ctx, m.control, m.service, "DeletePortMapping", [][2]string{
		{"NewRemoteHost", ""}, {"NewExternalPort", strconv.Itoa(int(m.External.Port()))}, {"NewProtocol", "TCP"},
	})
	return err
}
