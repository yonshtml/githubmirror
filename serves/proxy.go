package serves

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/andybalholm/brotli"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
	"golang.org/x/net/html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"seo_mirror/models"
	"sort"
	"strings"
	"text/template"
	"time"
)

const (
	headerXProxyTarget = "X-Proxy-target"
	headerXOriginHost  = "X-Origin-Host"
	headerXProxyClient = "X-Proxy-Client"
)

type proxyServer struct {
	*proxyServerOptions
	*AppServe
	proxy *httputil.ReverseProxy
}

type proxyServerOptions struct {
	target               *url.URL
	useSSL               bool
	reqHeaders           http.Header
	resHeaders           http.Header
	proxyExternal        bool
	proxyExternalIgnores []string
	cors                 bool
	noCache              bool
	cacheFolder          string
	sourceDomain         string
}

func newProxyServer(options *proxyServerOptions, app *AppServe) *proxyServer {
	server := &proxyServer{
		proxyServerOptions: options,
		AppServe:           app,
	}
	return server
}

func (p *proxyServer) initProxy(remoteUrl *url.URL) {
	p.target = remoteUrl
	p.proxy = httputil.NewSingleHostReverseProxy(remoteUrl)
	originalDirector := p.proxy.Director
	p.proxy.Director = func(req *http.Request) {
		originalDirector(req)
		p.modifyRequest(req)
	}
	//proxy, _ := url.Parse(p.Conf.Serve.WebProxy)
	//p.Logger.Info("当前使用代理ip", p.Conf.Serve.WebProxy)
	proxyUrl, _ := url.Parse(p.Conf.Serve.WebProxy)
	proxy := http.ProxyURL(proxyUrl)
	//p.Logger.Info("当前使用代理ip", p.Conf.Serve.WebProxy)
	if p.Conf.Serve.WebProxy == "" {
		proxy = nil
	}
	http.DefaultTransport.(*http.Transport).Clone()
	p.proxy.Transport = &http.Transport{
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
		Proxy:                 proxy,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableKeepAlives:     true,
	}
	p.proxy.ModifyResponse = p.modifyResponse
	p.proxy.ErrorHandler = func(rw http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.Canceled) {
			return
		}
		p.Logger.Errorf("%+v\n", err.Error())
		rw.WriteHeader(http.StatusInternalServerError)
		_, _ = rw.Write([]byte("请求原始网址出错：" + err.Error()))
	}
}

func clientIP(r *http.Request) string {
	xForwardedFor := r.Header.Get("X-Forwarded-For")
	ip := strings.TrimSpace(strings.Split(xForwardedFor, ",")[0])
	if ip != "" {
		return ip
	}
	ip = strings.TrimSpace(r.Header.Get("X-Real-Ip"))
	if ip != "" {
		return ip
	}
	if ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr)); err == nil {
		return ip
	}
	return ""
}

// Handler 所有请求处理
func (p *proxyServer) Handler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		//w.Header().Set("Access-Control-Allow-Origin", "*")
		//w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		//w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		ua := r.UserAgent()
		ip := clientIP(r)
		if p.adJump(ua, r.URL.Path, r.Host, ip) {
			t, _ := template.ParseFiles("public/404.html")
			_ = t.Execute(w, nil)
			return
		}
		p.sourceDomain = r.Host
		websiteData, ok := p.siteConfigData[r.Host]
		if !ok {
			// 查询是否还有缓存数据
			if p.queueData.Any() {
				//读取一条数据出来
				p.siteConfigData[r.Host] = p.queueData.Pop().(models.WebSiteConfig)
				*p.siteConfigData[r.Host].Domain = r.Host
				// 查询库里面是否定义
				_, err := p.Db.GetOne(r.Host)
				if err == nil {
					p.ResetData()
					websiteData = p.siteConfigData[r.Host]
				} else {
					err = p.Db.UpdateById(p.siteConfigData[r.Host])
					if err != nil {
						p.Logger.Error("更新出错")
					}
					websiteData = p.siteConfigData[r.Host]
				}
			} else {
				w.WriteHeader(502)
				_, _ = w.Write([]byte(fmt.Sprintf("%+v\n", "502 网站数据没有准备好!")))
				return
			}
		}
		//将原始 url 解析为 URL 结构
		remoteUrl, err := url.Parse(websiteData.OrgUrl)
		if err != nil {
			p.Logger.Error("目标解析失败:", err)
			return
		}
		p.initProxy(remoteUrl)
		//timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		//hash := md5.Sum([]byte("orderno=ZF202212185700Byedmx,secret=6be0d512ba6246cbbce651b364a357ca,timestamp=" + timestamp))
		//sign := hex.EncodeToString(hash[:])
		//auth := "sign=" + sign + "&orderno=ZF202212185700Byedmx&timestamp=" + timestamp
		//hdr := http.Header{}
		//hdr.Add("Proxy-Authorization", auth)
		//r.Header.Set("1122333", auth)
		r.Header.Set("Origin-Ua", ua)
		if p.isCrawler(ua) && !p.isGoodCrawler(ua) {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(fmt.Sprintf("%+v\n", "404 页面未找到!")))
			return
		}
		if p.cacheFolder != "" && r.Method == http.MethodGet && websiteData.WebCacheEnable == 1 {
			p.getCache(w, r, websiteData)
		} else {
			p.Logger.Infof("非正常请求%+v,%+v,请求方式%+v", *websiteData.Domain, remoteUrl, r.Method)
			//t, _ := template.ParseFiles("public/404.html")
			//_ = t.Execute(w, nil)
			w.WriteHeader(403)
			_, _ = w.Write([]byte(fmt.Sprintf("%+v\n", "403 页面未找到!")))
			return
			//w.Header().Set("Cache-Control", "must-revalidate, no-store")
			//w.Header().Set("Content-Type", " text/html;charset=UTF-8")
			//w.Header().Set("Location", "http://www.baidu.com/") //跳转地址设置
			//w.WriteHeader(http.StatusFound)
			//p.proxy.ServeHTTP(w, r)
		}
	}
}

func (p *proxyServer) adJump(ua, urlPath string, host string, ip string) bool {
	// 蜘蛛头部
	reSpider := regexp.MustCompile(`Baiduspider|360Spider|Bytespider|Sogou|Yisou|bingbot|Googlebot`)
	// 手机端ua
	reMobile := regexp.MustCompile(`iPhone|iPod|Android|ios|iOS|iPad|WebOS|Symbian|Windows Phone|Phone`)
	// 主域名
	reDomain := regexp.MustCompile(`www\.|m\.`)
	// 域名后缀
	reRootDomain := regexp.MustCompile(`[^\.]*\.(com\.cn|com\.tw|cc|cn|aero|arpa|asia|biz|cat|com|coop|edu|gov|int|info|jobs|mil|mobi|museum|name|net|org|pro|tel|trave)`)
	// 判断蜘蛛头部
	matchSpider := reSpider.FindStringSubmatch(ua)
	// 判断手机端ua
	matchMobile := reMobile.FindStringSubmatch(ua)
	// 判断主域名
	matchDomain := reDomain.FindStringSubmatch(host)
	// 判断域名后缀
	rootDomain := reRootDomain.FindStringSubmatch(host)
	// 是否是首页
	isIndex := urlIsIndex(urlPath)
	// 是否是图片
	img := isImg(filepath.Ext(urlPath))
	// 是否是静态资源
	static := isStatic(filepath.Ext(urlPath))
	// 如果不是蜘蛛ip全部跳转
	if p.Conf.Serve.SimulateSpiders == 1 {
		index := sort.SearchStrings(p.AppServe.spider, ip)
		if index < len(p.AppServe.spider) && p.AppServe.spider[index] == ip {
			return false
		} else {
			return true
		}
	}
	switch p.Conf.Serve.JumpType {
	case 1: //不是蜘蛛就跳转
		if len(matchSpider) == 0 && (!isIndex || (len(matchDomain) == 0 && rootDomain[0] != host)) && !img && !static {
			return true
		}
		// 如果开启了首页跳转，并且是主域名或者是没后后面路径  并且是首页跳转
		if p.Conf.Serve.JumpIndex == 1 && len(matchSpider) == 0 && (len(matchDomain) > 0 || rootDomain[0] == host) && isIndex {
			return true
		}
	case 2: //手机端就跳转
		if len(matchMobile) > 0 && len(matchSpider) == 0 && (!isIndex || (len(matchDomain) == 0 && rootDomain[0] != host)) && !img && !static {
			return true
		}
		// 如果开启了首页跳转，并且是主域名或者是没后后面路径  并且是首页跳转
		if p.Conf.Serve.JumpIndex == 1 && len(matchSpider) == 0 && (len(matchDomain) > 0 || rootDomain[0] == host) && isIndex {
			return true
		}
	default:
		return false
	}
	return false
}

func (p *proxyServer) getCache(w http.ResponseWriter, r *http.Request, c models.WebSiteConfig) bool {
	paths := []string{p.cacheFolder, r.Host}
	path := r.URL.Path
	if isIndexPage(path) {
		path = "/index.html"
	}
	paths = append(paths, strings.Split(strings.TrimLeft(path, "/"), "/")...)
	cacheFilePath := filepath.Join(paths...)
	cacheFilePathName := cacheFilePath + ".cache"
	if isImg(filepath.Ext(path)) && p.Conf.Serve.SaveImages == true {
		cacheFilePathName = cacheFilePath
	}
	// 读取缓存信息
	fInfo, err := os.Stat(cacheFilePathName)
	if os.IsNotExist(err) {
		p.Logger.Info("没有缓存信息，请求原始网站", c.OrgUrl, r.URL)
		p.proxy.ServeHTTP(w, r)
		return true
	}
	if err != nil {
		if strings.Contains(err.Error(), "文件名太长") {
			p.Logger.Warn("缓存文件名太长", cacheFilePath)
			p.proxy.ServeHTTP(w, r)
			return true
		} else {
			p.Logger.Warn("读取缓存文件信息错误", cacheFilePath)
			w.WriteHeader(http.StatusMethodNotAllowed)
			w.Write([]byte(fmt.Sprintf("%+v\n", "404 页面未找到!")))
			return true
		}
	}
	modTime := fInfo.ModTime()
	if time.Now().Unix() > modTime.Unix()+c.WebCacheTime*60 {
		p.Logger.Info("缓存过期，请求原始网站", cacheFilePath)
		p.proxy.ServeHTTP(w, r)
		return true
	}
	if fInfo.IsDir() {
		p.Logger.Warn("缓存文件是一个目录", cacheFilePath)
		p.proxy.ServeHTTP(w, r)
		return true
	}
	p.Logger.Infof("开始读取缓存%+v,%+v", c.OrgUrl, r.URL)
	f, err := os.Open(cacheFilePathName)
	if err != nil {
		p.Logger.Error("打开缓存文件错误")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(fmt.Sprintf("%+v\n", errors.WithStack(err))))
		return true
	}
	defer f.Close()

	MIMEType := mime.TypeByExtension(filepath.Ext(cacheFilePath))
	if strings.Contains(MIMEType, "php") || strings.Contains(MIMEType, "asp") {
		MIMEType = "text/html; charset=utf-8"
	}
	w.Header().Set("Content-Type", MIMEType)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, f)
	return false
}

func (p *proxyServer) modifyRequest(req *http.Request) {
	target := *p.target
	isProxyUrl := req.URL.Query().Get("proxy_url") != ""

	if isProxyUrl {
		if unescapeUrl, err := url.QueryUnescape(strings.TrimLeft(req.URL.RawQuery, "proxy_url=")); err == nil {
			if u, err := url.Parse(unescapeUrl); err == nil {
				if u.Scheme == "" {
					if p.useSSL {
						u.Scheme = "https"
					} else {
						u.Scheme = "http"
					}
				}
				target = *u
			}
		}
	} else if req.Header.Get(headerXProxyTarget) != "" {
		targetUrl := req.Header.Get(headerXProxyTarget)

		if u, err := url.Parse(targetUrl); err == nil {
			if u.Scheme == "" {
				if p.useSSL {
					u.Scheme = "https"
				} else {
					u.Scheme = "http"
				}
			}
			target = *u
		}
	}

	req.Header.Set(headerXOriginHost, req.Host)
	req.Host = target.Host
	if isProxyUrl {
		req.URL = &target
	} else {
		req.URL.Host = target.Host
		req.URL.Scheme = target.Scheme
	}

	req.Header.Set("Host", target.Host)
	req.Header.Set("Origin", fmt.Sprintf("%s://%s", target.Scheme, target.Host))
	//req.Header.Set("Referrer", fmt.Sprintf("%s://%s%s", target.Scheme, target.Host, req.URL.RawPath))
	req.Header.Set("X-Real-IP", req.RemoteAddr)

	for k := range p.reqHeaders {
		req.Header.Add(k, p.reqHeaders.Get(k))
	}
}

func (p *proxyServer) modifyContent(extNames []string, body []byte, originHost, proxyHost, path string) []byte {
	bodyStr := string(body)
	bodyStr = replaceHost(bodyStr, originHost, proxyHost, p.useSSL, p.proxyExternal, p.proxyExternalIgnores)
	if isHtml(extNames) {
		isIndexPage := isIndexPage(path)

		bodyStr = regIntegrity.ReplaceAllString(bodyStr, "")

		bodyStr = strings.ReplaceAll(bodyStr, `http-equiv="Content-Security-Policy"`, "")
		site := p.siteConfigData[proxyHost]
		if site.WebReplaces != "" {
			bodyStr = replaceWebReplaces(p, site.WebReplaces, bodyStr)
		}

		if !isIndexPage {
			bodyStr = insertTag(p, bodyStr, site.WebTitle)
		}

		if *site.WebFriendLink != "" && isIndexPage {
			bodyStr = replaceFriendLink(p, html.UnescapeString(*site.WebFriendLink), bodyStr)
		}

		if *site.WebDate != "" {
			bodyStr = replaceWebDate(p, *site.WebDate, bodyStr)
		}

		if *site.WebImage != "" {
			bodyStr = replaceWebImage(p, html.UnescapeString(*site.WebImage), bodyStr)
		}

		if *site.WebRandomHref != "" {
			bodyStr = replaceWebRandomHref(p, html.UnescapeString(*site.WebRandomHref), bodyStr)
		}

		if *site.WebRandomContent != "" {
			bodyStr = replaceWebRandomContent(p, html.UnescapeString(*site.WebRandomContent), bodyStr)
		}

		if site.WebS2t == 1 {
			bodyStr, _ = p.S2T.ConvertText(bodyStr)
		}

		document, err := html.Parse(bytes.NewReader([]byte(bodyStr)))
		if err != nil {
			p.Logger.Errorf("解析html错误：%v", err)
		}
		for c := document.FirstChild; c != nil; c = c.NextSibling {
			p.handleHtmlNode(c, isIndexPage, site)

		}
		var buf bytes.Buffer
		err = html.Render(&buf, document)
		if err != nil {
			p.Logger.Error("html渲染错误", err.Error())
			return []byte(bodyStr)
		}
		return buf.Bytes()

	}
	return []byte(bodyStr)
}

func (p *proxyServer) modifyResponse(res *http.Response) error {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return errors.WithStack(err)
	}
	defer res.Body.Close()
	if len(body) < 500 {
		p.Logger.Infof("返回请求体太小%v,返回数据%+v", len(body), string(body))
		res.Header.Set("Content-Length", fmt.Sprint(len(body)))
		res.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}
	if res.StatusCode == 404 {
		res.Header.Set("Content-Length", fmt.Sprint(len(body)))
		res.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}
	path := strings.TrimRight(res.Request.URL.Path, "/")
	proxyHost := res.Request.Header.Get(headerXOriginHost)
	if isImg(filepath.Ext(path)) && p.Conf.Serve.SaveImages == true {
		p.Logger.Info("图片文件开始下载", path)
		err = models.SetImgCache(proxyHost+path, body, p.cacheFolder)
		if err != nil {
			return errors.WithStack(err)
		}
		res.Header.Set("Content-Length", fmt.Sprint(len(body)))
		res.Body = io.NopCloser(bytes.NewReader(body))
		return nil
	}
	target := *p.target

	isProxyUrl := res.Request.URL.Query().Get("proxy_url") != ""
	if isProxyUrl {
		if unescapeUrl, err := url.QueryUnescape(strings.TrimLeft(res.Request.URL.RawQuery, "proxy_url=")); err == nil {
			if u, err := url.Parse(unescapeUrl); err == nil {
				if p.useSSL {
					u.Scheme = "https"
				} else {
					u.Scheme = "http"
				}
				target = *u
			}
		}
	}
	var hostName string
	if strings.Contains(proxyHost, ":") {
		h, _, err := net.SplitHostPort(proxyHost)
		if err != nil {
			return errors.WithStack(err)
		}
		hostName = h
	} else {
		hostName = proxyHost
	}
	res.Header.Set(headerXProxyClient, "Forward-Cli")
	res.Header.Del("Expect-CT")

	p.disableCache(res)

	p.disableCsp(res)

	p.disableSts(res)

	p.reHttpCode(res)

	p.reCookie(res, hostName)

	p.redirect(res, isProxyUrl, target, proxyHost)

	if p.cors {
		res.Header.Set("Access-Control-Allow-Origin", "*")
		res.Header.Set("Access-Control-Allow-Credentials", "true")
	}

	for k := range p.resHeaders {
		res.Header.Add(k, p.resHeaders.Get(k))
	}

	{
		contentType := res.Header.Get("Content-Type")
		extNames, err := mime.ExtensionsByType(contentType)
		if err != nil {
			return nil
		}
		if !isShouldReplaceContent(extNames) || isImg(filepath.Ext(path)) {
			return nil
		}
		encoding := res.Header.Get("Content-Encoding")
		switch encoding {
		case "gzip":
			reader, err := gzip.NewReader(res.Body)
			if err != nil {
				return errors.WithStack(err)
			}
			defer reader.Close()
			body, err := io.ReadAll(reader)
			if err != nil {
				return errors.WithStack(err)
			}
			body = GBk2UTF8(body, contentType)
			newBody := body
			if strings.Contains(contentType, "html") {
				newBody = p.modifyContent(extNames, body, target.Host, proxyHost, path)
			} else if strings.Contains(contentType, "xml") {
				newBody = []byte(replaceHost(string(body), target.Host, proxyHost, p.useSSL, p.proxyExternal, p.proxyExternalIgnores))
			}
			var b bytes.Buffer
			gz := gzip.NewWriter(&b)
			if _, err := gz.Write(newBody); err != nil {
				return errors.WithStack(err)
			}
			if err := gz.Close(); err != nil {
				return errors.WithStack(err)
			}
			bin := b.Bytes()
			res.Header.Set("Content-Length", fmt.Sprint(len(bin)))
			res.Body = io.NopCloser(bytes.NewReader(bin))
			if isIndexPage(res.Request.URL.Path) {
				path = "/index.html"
			}
			err = models.SetCache(proxyHost+path, res, bin, p.cacheFolder)
			if err != nil {
				return errors.WithStack(err)
			}
		case "deflate":
			reader, err := zlib.NewReader(res.Body)
			if err != nil {
				return errors.WithStack(err)
			}
			body, err := io.ReadAll(reader)
			defer reader.Close()
			if err != nil {
				return errors.WithStack(err)
			}
			newBody := p.modifyContent(extNames, body, target.Host, proxyHost, path)
			buf := &bytes.Buffer{}
			w := zlib.NewWriter(buf)
			if n, err := w.Write(newBody); err != nil {
				return errors.WithStack(err)
			} else if n < len(newBody) {
				return fmt.Errorf("读取到数据太小: %d vs %d for %s", n, len(newBody), string(newBody))
			}
			if err := w.Close(); err != nil {
				return errors.WithStack(err)
			}
			res.Header.Set("Content-Length", fmt.Sprint(buf.Len()))
			res.Body = io.NopCloser(buf)
		case "br":
			reader := brotli.NewReader(res.Body)
			body, err := io.ReadAll(reader)
			if err != nil {
				return errors.WithStack(err)
			}
			p.Logger.Infof("br ,url :%s", res.Request.URL.Path+res.Request.URL.RawQuery)
			newBody := p.modifyContent(extNames, body, target.Host, proxyHost, path)
			buf := &bytes.Buffer{}
			w := brotli.NewWriter(buf)
			if n, err := w.Write(newBody); err != nil {
				return errors.WithStack(err)
			} else if n < len(newBody) {
				return fmt.Errorf("n too small: %d vs %d for %s", n, len(newBody), string(newBody))
			}
			if err := w.Close(); err != nil {
				return errors.WithStack(err)
			}
			res.Header.Set("Content-Length", fmt.Sprint(buf.Len()))
			res.Body = io.NopCloser(buf)
		case "identity":
			fallthrough
		default:
			body = GBk2UTF8(body, contentType)
			cType := contentType
			byteBody := body

			if strings.Contains(cType, "html") {
				newBody := p.modifyContent(extNames, body, target.Host, proxyHost, path)
				res.Header.Set("Content-Length", fmt.Sprint(len(newBody)))
				res.Body = io.NopCloser(bytes.NewReader(newBody))
				byteBody = newBody
			} else if strings.Contains(cType, "xml") {
				newBody := []byte(replaceHost(string(body), target.Host, proxyHost, p.useSSL, p.proxyExternal, p.proxyExternalIgnores))
				res.Header.Set("Content-Length", fmt.Sprint(len(newBody)))
				res.Body = io.NopCloser(bytes.NewReader(newBody))
				byteBody = newBody
			} else {
				res.Header.Set("Content-Length", fmt.Sprint(len(body)))
				res.Body = io.NopCloser(bytes.NewReader(body))
			}
			if isIndexPage(res.Request.URL.Path) {
				path = "/index.html"
			}
			err = models.SetCache(proxyHost+path, res, byteBody, p.cacheFolder)
			if err != nil {
				return errors.WithStack(err)
			}
		}

	}

	return nil
}

func (p *proxyServer) redirect(res *http.Response, isProxyUrl bool, target url.URL, proxyHost string) {
	{

		location := res.Header.Get("Location")
		if location != "" {
			if !isHttpUrl(location) {
				if isProxyUrl {
					newLocation := target
					newLocation.Path = location
					res.Header.Set("Location", newLocation.String())
				}
			} else {
				newLocation := replaceHost(location, target.Host, proxyHost, p.useSSL, p.proxyExternal, p.proxyExternalIgnores)
				res.Header.Set("Location", newLocation)
			}
		}
	}
}

func (p *proxyServer) reCookie(res *http.Response, hostName string) {
	{
		cookies := res.Cookies()
		res.Header.Del("Set-Cookie")

		for _, v := range cookies {
			v.Domain = hostName
			if v.Secure && !p.useSSL {
				v.Secure = false
			}

			res.Header.Add("Set-Cookie", v.String())
		}
	}
}

func (p *proxyServer) reHttpCode(res *http.Response) {
	{

		if res.StatusCode == http.StatusMovedPermanently {
			res.StatusCode = http.StatusFound
		}
	}
}

func (p *proxyServer) disableSts(res *http.Response) {
	if !p.useSSL {

		res.Header.Del("Strict-Transport-Security")
	}
}

func (p *proxyServer) disableCsp(res *http.Response) {
	{

		res.Header.Del("Content-Security-Policy")
	}
}

func (p *proxyServer) disableCache(res *http.Response) {
	{
		if p.noCache {
			res.Header.Set("Cache-Control", "no-cache")
		}
	}
}

func (p *proxyServer) handleHtmlNode(node *html.Node, isIndexPage bool, webSite models.WebSiteConfig) {
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		switch c.Type {

		case html.ElementNode:
			switch c.Data {
			case "title":
				p.replaceTitle(c, isIndexPage, webSite)
			case "meta":
				p.replaceMeta(c, isIndexPage, webSite)
			}

		}
		p.handleHtmlNode(c, isIndexPage, webSite)
	}
}

func (p *proxyServer) isCrawler(ua string) bool {

	ua = strings.ToLower(ua)

	for _, value := range p.Conf.Spider {
		spider := strings.ToLower(value)

		if strings.Contains(ua, spider) {
			return true
		}
	}
	return false
}

func (p *proxyServer) isGoodCrawler(ua string) bool {
	ua = strings.ToLower(ua)
	for _, value := range p.Conf.GoodSpider {
		spider := strings.ToLower(value)
		if strings.Contains(ua, spider) {
			return true
		}
	}
	return false
}

func (p *proxyServer) replaceTitle(node *html.Node, isIndexPage bool, webConf models.WebSiteConfig) {
	if isIndexPage {
		title := webConf.WebSeoTitle
		if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
			node.FirstChild.Data = title
			return
		}
		node.FirstChild = &html.Node{
			Type: html.TextNode,
			Data: title,
		}
		return
	}
	if *webConf.ContentTitle != "" && !isIndexPage {
		var replaceInfo map[string]string
		if err := json.Unmarshal([]byte(*webConf.ContentTitle), &replaceInfo); err == nil {
			title := replaceInfo["title"]
			if node.FirstChild != nil && node.FirstChild.Type == html.TextNode {
				node.FirstChild.Data = title
				return
			}
			node.FirstChild = &html.Node{
				Type: html.TextNode,
				Data: title,
			}
		} else {
			p.Logger.Errorf("replaceWebRandomContent 数据转换出错：%v", err)
		}
	}
}

func (p *proxyServer) replaceMeta(node *html.Node, isIndexPage bool, webConf models.WebSiteConfig) {
	content := ""
	for i, attr := range node.Attr {
		if attr.Key == "name" && isIndexPage {
			if attr.Val == "keywords" {
				content = webConf.WebKeywords
				break
			}
			if attr.Val == "description" {
				content = webConf.WebDescription
				break
			}
		}

		if attr.Key == "name" && !isIndexPage {
			var replaceInfo map[string]string
			if err := json.Unmarshal([]byte(*webConf.ContentTitle), &replaceInfo); err == nil {
				if attr.Val == "keywords" {
					content = replaceInfo["keywords"]
					break
				}
				if attr.Val == "description" {
					content = replaceInfo["description"]
					break
				}
			} else {
				p.Logger.Errorf("replaceWebRandomContent 数据转换出错：%v", err)
			}
		}

		if strings.ToLower(attr.Key) == "http-equiv" && strings.ToLower(attr.Val) == "content-type" {
			content = "text/html; charset=UTF-8"
			break
		}
		if attr.Key == "charset" {
			node.Attr[i].Val = "UTF-8"
		}
	}
	if content == "" {
		return
	}
	for i, attr := range node.Attr {
		if attr.Key == "content" {
			node.Attr[i].Val = content
		}
	}

}

func (p *proxyServer) showPicHandle(w http.ResponseWriter, req *http.Request) {
	file, err := os.Open("." + req.URL.Path)
	if err != nil {
		p.Logger.Error("图片读取错误", err)
		return
	}
	defer file.Close()
	buff, err := io.ReadAll(file)
	if err != nil {
		p.Logger.Error("图片读取错误", err)
		return
	}
	w.Write(buff)
}
