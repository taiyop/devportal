package main

import (
	"errors"
	"net"
	"syscall"
)

func listenGateway(port string, lan []gatewayLANAddress) ([]net.Listener, error) {
	v4, err := net.Listen("tcp4", net.JoinHostPort(loopbackV4, port))
	if err != nil {
		return nil, err
	}
	listeners := []net.Listener{v4}
	port = itoaPort(tcpListenPort(v4))
	v6, err := net.Listen("tcp6", net.JoinHostPort(loopbackV6, port))
	if err == nil {
		listeners = append(listeners, v6)
	} else if !errors.Is(err, syscall.EAFNOSUPPORT) && !errors.Is(err, syscall.EADDRNOTAVAIL) && !errors.Is(err, syscall.EPROTONOSUPPORT) {
		_ = v4.Close()
		return nil, err
	}

	for _, local := range lan {
		listener, err := net.Listen("tcp", net.JoinHostPort(local.host, port))
		if err != nil {
			for _, opened := range listeners {
				_ = opened.Close()
			}
			return nil, err
		}
		listeners = append(listeners, gatewayLANListener{Listener: listener, subnet: local.subnet})
	}
	return listeners, nil
}

type gatewayLANAddress struct {
	host   string
	subnet *net.IPNet
}

func gatewayLANAddresses() ([]gatewayLANAddress, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var lan []gatewayLANAddress
	seen := make(map[string]bool)
	for _, iface := range interfaces {
		if iface.Flags&(net.FlagUp|net.FlagBroadcast) != (net.FlagUp|net.FlagBroadcast) || iface.Flags&(net.FlagLoopback|net.FlagPointToPoint) != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}
		for _, addr := range addrs {
			subnet, ok := addr.(*net.IPNet)
			if !ok || !isGatewayLANIP(subnet.IP) {
				continue
			}
			host := subnet.IP.String()
			if subnet.IP.To4() == nil && subnet.IP.IsLinkLocalUnicast() {
				host += "%" + iface.Name
			}
			if !seen[host] {
				lan = append(lan, gatewayLANAddress{host: host, subnet: subnet})
				seen[host] = true
			}
		}
	}
	return lan, nil
}

func isGatewayLANIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

type gatewayLANListener struct {
	net.Listener
	subnet *net.IPNet
}

func (l gatewayLANListener) allows(peer net.IP) bool {
	return isGatewayLANIP(peer) && l.subnet.Contains(peer)
}

func (l gatewayLANListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		peer := conn.RemoteAddr().(*net.TCPAddr)
		if l.allows(peer.IP) {
			return conn, nil
		}
		_ = conn.Close()
	}
}
