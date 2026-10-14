package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
)

// Downloads de URLs que vêm do cliente da API (mídia, link preview, foto do
// perfil, foto do grupo, carrossel, produto). Com http.Get puro, quem chama a
// API faz o servidor acessar a rede interna (SSRF): localhost, outros
// containers, o endereço de metadados da nuvem. O SafeHTTPClient confere o IP
// NA HORA DA CONEXÃO — depois do DNS e também em cada redirect —, então um
// nome que resolve (ou passa a resolver) para IP interno é recusado.
//
// ALLOW_PRIVATE_URLS=true desliga a checagem, para quem serve mídia de um
// storage na rede interna (ex.: MinIO no mesmo docker-compose).

// ErrPrivateAddress é devolvido quando a URL aponta para endereço interno.
var ErrPrivateAddress = errors.New("url points to a private or reserved address")

// safeDownloadTimeout cobre o download inteiro (inclui vídeos grandes).
const safeDownloadTimeout = 120 * time.Second

var reservedNets = func() []*net.IPNet {
	var nets []*net.IPNet
	for _, cidr := range []string{
		"0.0.0.0/8",       // "esta rede"
		"100.64.0.0/10",   // CGNAT
		"192.0.0.0/24",    // IETF
		"192.0.2.0/24",    // documentação
		"198.18.0.0/15",   // benchmark
		"198.51.100.0/24", // documentação
		"203.0.113.0/24",  // documentação
		"240.0.0.0/4",     // reservado (inclui broadcast)
		"64:ff9b::/96",    // NAT64 (embute IPv4)
		"2001:db8::/32",   // documentação
	} {
		_, n, _ := net.ParseCIDR(cidr)
		nets = append(nets, n)
	}
	return nets
}()

// IsPublicIP diz se o IP é roteável na internet (nem privado, nem loopback,
// link-local, multicast ou reservado).
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4 // IPv4 mapeado em IPv6 (::ffff:10.0.0.1) cai nas regras de IPv4
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return false
	}
	for _, n := range reservedNets {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

func allowPrivateURLs() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_PRIVATE_URLS")), "true")
}

// dialControl roda depois do DNS, com o IP que vai ser de fato conectado.
func dialControl(_, address string, _ syscall.RawConn) error {
	if allowPrivateURLs() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if !IsPublicIP(net.ParseIP(host)) {
		return ErrPrivateAddress
	}
	return nil
}

func newSafeHTTPClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second, Control: dialControl}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			// Conexão direta: um proxy de saída (HTTP_PROXY) na rede interna
			// seria recusado pela própria checagem.
			Proxy:                 nil,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

// SafeHTTPClient é o cliente para baixar URLs informadas por quem chama a API.
var SafeHTTPClient = newSafeHTTPClient(safeDownloadTimeout)

// SafeGet é o substituto de http.Get para URLs vindas do cliente da API.
func SafeGet(url string) (*http.Response, error) {
	return SafeHTTPClient.Get(url)
}

// DownloadImage baixa uma imagem com prazo e tamanho máximos, exigindo
// Content-Type image/* (quando o servidor informa um).
func DownloadImage(ctx context.Context, url string, maxBytes int64, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid image url: %w", err)
	}
	resp, err := SafeHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("image download returned status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "image/") {
		return nil, fmt.Errorf("url is not an image (content-type %q)", ct)
	}
	if resp.ContentLength > maxBytes {
		return nil, fmt.Errorf("image larger than %d bytes", maxBytes)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("image larger than %d bytes", maxBytes)
	}
	return data, nil
}
