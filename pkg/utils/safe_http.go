package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"strconv"
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

// Recusas do download que dependem só da URL: re-tentar dá o mesmo resultado.
// O handler responde 422 para elas (IsMediaURLRefused).
var (
	ErrTooManyRedirects = errors.New("too many redirects")
	ErrRedirectScheme   = errors.New("redirect to a non-https URL")
	ErrMediaTooLarge    = errors.New("media larger than the download limit")
)

// safeDownloadTimeout cobre o download inteiro (inclui vídeos grandes).
const safeDownloadTimeout = 120 * time.Second

// maxRedirects: cada redirect passa de novo pela checagem de IP na conexão.
const maxRedirects = 3

// defaultMediaDownloadMaxMB vale quando MEDIA_DOWNLOAD_MAX_MB não está definida.
// O WhatsApp aceita documento bem maior, mas baixar sem teto deixa uma URL de
// arquivo enorme esgotar a memória do servidor.
const defaultMediaDownloadMaxMB = 100

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
		"::/96",           // IPv4-compatível (::7f00:1), além de :: e ::1
		"64:ff9b::/96",    // NAT64 (embute IPv4)
		"64:ff9b:1::/48",  // NAT64 local
		"100::/64",        // descarte
		"2001::/32",       // Teredo (embute IPv4)
		"2001:db8::/32",   // documentação
		"2002::/16",       // 6to4 (embute IPv4)
		"fec0::/10",       // site-local (obsoleto, ainda roteado em redes antigas)
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
		CheckRedirect: checkRedirect,
	}
}

// checkRedirect: no máximo maxRedirects, só http(s) e nunca de https para
// http (o destino interno costuma ser http, ex. o endereço de metadados).
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return ErrTooManyRedirects
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return ErrRedirectScheme
	}
	if req.URL.Scheme == "http" && len(via) > 0 && via[len(via)-1].URL.Scheme == "https" {
		return ErrRedirectScheme
	}
	return nil
}

// SafeHTTPClient é o cliente para baixar URLs informadas por quem chama a API.
var SafeHTTPClient = newSafeHTTPClient(safeDownloadTimeout)

// SafeGet é o substituto de http.Get para URLs vindas do cliente da API.
func SafeGet(url string) (*http.Response, error) {
	return SafeHTTPClient.Get(url)
}

// MediaDownloadMaxBytes é o teto de um download de mídia
// (MEDIA_DOWNLOAD_MAX_MB, padrão 100). Lida a cada chamada, como
// ALLOW_PRIVATE_URLS.
func MediaDownloadMaxBytes() int64 {
	mb := defaultMediaDownloadMaxMB
	if raw := strings.TrimSpace(os.Getenv("MEDIA_DOWNLOAD_MAX_MB")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			mb = v
		}
	}
	return int64(mb) << 20
}

// ReadLimited lê o corpo até maxBytes. Recusa pelo Content-Length declarado e
// também contando o que chega (o header pode mentir ou faltar).
func ReadLimited(resp *http.Response, maxBytes int64) ([]byte, error) {
	if resp.ContentLength > maxBytes {
		return nil, ErrMediaTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrMediaTooLarge
	}
	return data, nil
}

// DownloadMedia baixa a mídia de uma URL informada por quem chama a API, com o
// teto de MediaDownloadMaxBytes.
func DownloadMedia(ctx context.Context, rawURL string) ([]byte, error) {
	return DownloadLimited(ctx, rawURL, MediaDownloadMaxBytes())
}

// DownloadLimited: checagem de IP na conexão, redirects limitados, status 2xx
// e teto de tamanho. O erro NÃO repete a URL (ela pode ter token na query e
// voltaria ao cliente na resposta) e começa com "media download failed".
func DownloadLimited(ctx context.Context, rawURL string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, errors.New("media download failed: invalid url")
	}
	req.Header.Set("User-Agent", "Evolution-GO/1.0")
	resp, err := SafeHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("media download failed: %w", stripURL(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("media download failed: HTTP status %d", resp.StatusCode)
	}
	data, err := ReadLimited(resp, maxBytes)
	if err != nil {
		return nil, fmt.Errorf("media download failed: %w", stripURL(err))
	}
	return data, nil
}

// stripURL tira o *url.Error (`Get "https://…": `) e devolve só a causa.
func stripURL(err error) error {
	var ue *neturl.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// IsMediaURLRefused: o download foi recusado pela política de URL (endereço
// interno, redirect, tamanho) — erro do pedido, não do servidor.
func IsMediaURLRefused(err error) bool {
	return errors.Is(err, ErrPrivateAddress) || errors.Is(err, ErrTooManyRedirects) ||
		errors.Is(err, ErrRedirectScheme) || errors.Is(err, ErrMediaTooLarge)
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
