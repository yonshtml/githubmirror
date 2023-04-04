package serves

import (
	"golang.org/x/net/html/charset"
	"net"
	"regexp"
	"strings"

	xurls "mvdan.cc/xurls/v2"
)

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

var (
	regIntegrity        = regexp.MustCompile(`\sintegrity="[^"]+"`)
	urlWithSchemeRegExp *regexp.Regexp
	//hostNameRegexp      = regexp.MustCompile(`^(([a-zA-Z]{1})|([a-zA-Z]{1}[a-zA-Z]{1})|([a-zA-Z]{1}[0-9]{1})|([0-9]{1}[a-zA-Z]{1})|([a-zA-Z0-9][a-zA-Z0-9-_]{1,61}[a-zA-Z0-9]))\.([a-zA-Z]{2,6}|[a-zA-Z0-9-]{2,30}\.[a-zA-Z]{2,3})$`)
	hostNameRegexp = regexp.MustCompile(`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`)

	rewriteContentExtNames = map[string]struct{}{
		".html":  {},
		".htm":   {},
		".xhtml": {},
		".xml":   {},
		".yml":   {},
		".yaml":  {},
		".css":   {},
		".js":    {},
		".jsm":   {},
		".txt":   {},
		".text":  {},
		".json":  {},
	}
	htmlExtNames = map[string]struct{}{
		".html":  {},
		".htm":   {},
		".xhtml": {},
	}

	imgExtNames = map[string]struct{}{
		".jpg":   {},
		".jpeg":  {},
		".ico":   {},
		".gif":   {},
		".webp":  {},
		".png":   {},
		".pjpeg": {},
		".JPG":   {},
	}

	staticExtNames = map[string]struct{}{
		".xml":  {},
		".yml":  {},
		".yaml": {},
		".css":  {},
		".js":   {},
		".jsm":  {},
		".txt":  {},
		".text": {},
		".json": {},
		".woff": {},
	}

	isIndex = map[string]struct{}{
		"":             {},
		"/":            {},
		"/index.php":   {},
		"/index.asp":   {},
		"/index.jsp":   {},
		"/index.htm":   {},
		"/index.html":  {},
		"/index.shtml": {},
		"/main.htm":    {},
	}
)

func init() {
	if u, err := xurls.StrictMatchingScheme("https?://|wss?://|//"); err != nil {
		panic(err)
	} else {
		urlWithSchemeRegExp = u
	}
}

func isHttpUrl(u string) bool {
	return regexp.MustCompile(`^https?:\/\/`).MatchString(u)
}

func isShouldReplaceContent(extNames []string) bool {
	for _, extName := range extNames {
		if _, ok := rewriteContentExtNames[extName]; ok {
			return true
		}
	}
	return false
}

func isHtml(extNames []string) bool {
	for _, extName := range extNames {
		if _, ok := htmlExtNames[extName]; ok {
			return true
		}
	}
	return false
}

func isImg(suffix string) bool {
	if _, ok := imgExtNames[suffix]; ok {
		return true
	}
	return false
}
func isStatic(suffix string) bool {
	if _, ok := staticExtNames[suffix]; ok {
		return true
	}
	return false
}
func urlIsIndex(suffix string) bool {
	if _, ok := isIndex[suffix]; ok {
		return true
	}
	return false
}

func isIndexPage(u string) bool {
	return u == "" ||
		strings.EqualFold(u, "/") ||
		strings.EqualFold(u, "/index.php") ||
		strings.EqualFold(u, "/index.asp") ||
		strings.EqualFold(u, "/index.jsp") ||
		strings.EqualFold(u, "/index.htm") ||
		strings.EqualFold(u, "/index.html") ||
		strings.EqualFold(u, "/index.shtml") ||
		strings.EqualFold(u, "/main.htm")

}

func getLocalIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")

	if err != nil {
		return []byte("0.0.0.0")
	}

	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP
}

func GBk2UTF8(content []byte, contentType string) []byte {
	e, name, _ := charset.DetermineEncoding(content, contentType)
	if strings.ToLower(name) != "utf-8" {
		content, _ = e.NewDecoder().Bytes(content)
	}
	return content
}
