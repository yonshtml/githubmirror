package serves

import (
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/dlclark/regexp2"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

func replaceHost(content, oldHost, newHost string, useSSL bool, proxyExternal bool, proxyExternalIgnores []string) string {
	newContent := urlWithSchemeRegExp.ReplaceAllStringFunc(content, func(s string) string {
		matchUrl, err := url.Parse(s)
		if err != nil {
			return s
		}

		if net.ParseIP(matchUrl.Hostname()) == nil && !hostNameRegexp.MatchString(matchUrl.Hostname()) {
			return s
		}
		{
			var query []string
			queryArr := strings.Split(matchUrl.RawQuery, "&")

			for _, q := range queryArr {
				arr := strings.Split(q, "=")
				key := arr[0]
				if len(arr) == 1 {
					if strings.Contains(q, "=") {
						query = append(query, key+"=")
					} else {
						query = append(query, key)
					}
				} else {
					escapedValue := strings.Join(arr[1:], "=")

					if unescapedValue, err := url.QueryUnescape(escapedValue); err == nil {
						escapedValue = url.QueryEscape(replaceHost(unescapedValue, oldHost, newHost, useSSL, proxyExternal, proxyExternalIgnores))
					} else {
						escapedValue = replaceHost(escapedValue, oldHost, newHost, useSSL, proxyExternal, proxyExternalIgnores)
					}

					query = append(query, key+"="+escapedValue)
				}
			}

			matchUrl.RawQuery = strings.Join(query, "&")
		}

		if matchUrl.Host != oldHost {
			if !proxyExternal {
				return s
			}

			if contains(proxyExternalIgnores, matchUrl.Host) {
				return s
			}
			if contains([]string{"http", "https"}, matchUrl.Scheme) && !isImg(filepath.Ext(matchUrl.Path)) {
				scheme := "http"
				if useSSL {
					scheme = "https"
				}
				return fmt.Sprintf("%s://%s/?proxy_url=%s", scheme, newHost, url.QueryEscape(matchUrl.String()))
			} else if contains([]string{"ws", "wss"}, matchUrl.Scheme) && !isImg(filepath.Ext(matchUrl.Path)) {
				scheme := "ws"
				if useSSL {
					scheme = "wss"
				}
				return fmt.Sprintf("%s://%s/?proxy_url=%s", scheme, newHost, url.QueryEscape(matchUrl.String()))
			}
			return s
		}

		if contains([]string{"http", "https"}, matchUrl.Scheme) {
			if useSSL {
				s = regexp.MustCompile(`^https?:\/\/`).ReplaceAllString(s, "https://")
			} else {
				s = regexp.MustCompile(`^https?:\/\/`).ReplaceAllString(s, "http://")
			}
		} else if contains([]string{"ws", "wss"}, matchUrl.Scheme) {
			if useSSL {
				s = regexp.MustCompile(`^wss?:\/\/`).ReplaceAllString(s, "wss://")
			} else {
				s = regexp.MustCompile(`^wss?:\/\/`).ReplaceAllString(s, "ws://")
			}
		}

		s = strings.Replace(s, oldHost, newHost, 1)

		return s
	})

	return newContent
}

func insertTag(p *proxyServer, html string, webTitle string) string {
	// 将 HTML 解析为 goquery.Document 对象
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		p.Logger.Error(err)
	}
	// 获取第一个 <p> 标签
	firstP := doc.Find("p").First()
	if firstP.Index() < 1 {
		firstP = doc.Find("div").Last()
	}
	// 在第一个 <p> 标签之前插入一个 <a> 标签
	firstP.BeforeHtml("<a href='/'>" + webTitle + "</a>")
	// 输出修改后的 HTML
	modifiedHTML, err := doc.Html()
	if err != nil {
		p.Logger.Error(err)
	}
	return modifiedHTML
}

func replaceFriendLink(p *proxyServer, link string, bodyStr string) string {
	var repl string
	linkList := strings.SplitAfter(link, "</a>")
	for _, link := range linkList {
		if find := strings.Contains(link, p.sourceDomain); !find {
			repl += strings.TrimSpace(link) + "\n"
		}
	}
	//re := regexp2.MustCompile(`<\/div>(?=(?:(?!<\/div>)[\s\S])*$)`, 0)
	//repl = repl + "</div>"
	//bodyStr, err := re.Replace(bodyStr, repl, -1, -1)
	*p.siteConfigData[p.sourceDomain].WebFriendLink = repl
	err := p.Db.UpdateFriendLink(p.siteConfigData[p.sourceDomain])
	if err != nil {
		p.Logger.Error("更新友情库错误", err)
	}
	re, _ := regexp.Compile(`<\/body>`)
	bodyStr = re.ReplaceAllString(bodyStr, repl+"</body>")
	return bodyStr
}

func replaceWebReplaces(p *proxyServer, WebReplaces string, bodyStr string) string {
	replaceInfo := map[string]string{}
	if err := json.Unmarshal([]byte(WebReplaces), &replaceInfo); err == nil {
		keys := make([]string, 0, len(replaceInfo))
		for k := range replaceInfo {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			return len(keys[i]) > len(keys[j])
		})
		for _, k := range keys {
			if find := strings.Contains(bodyStr, k); find {
				bodyStr = strings.ReplaceAll(bodyStr, k, replaceInfo[k])
			}
		}
	} else {
		p.Logger.Errorf("replaceWebReplaces 数据转换出错：%v", err)
	}
	return bodyStr
}

func replaceWebTitle(webTitle string, bodyStr string) string {
	re, _ := regexp.Compile(`<title>(.*?)<\/title>`)
	bodyStr = re.ReplaceAllString(bodyStr, "<title>"+webTitle+"</title>")
	return bodyStr
}

func replaceWebH1(webH1 string, bodyStr string) string {
	re, _ := regexp.Compile(`<h1>(.*?)<\/h1>`)
	bodyStr = re.ReplaceAllString(bodyStr, "<h1>"+webH1+"</h1>")
	return bodyStr
}

func replaceWebDate(p *proxyServer, webDate string, bodyStr string) string {
	dates := map[string][]map[string]string{}
	currentTime := time.Now()
	if err := json.Unmarshal([]byte(webDate), &dates); err == nil {
		for _, v := range dates["list"] {
			bodyStr = strings.ReplaceAll(bodyStr, v["position"], currentTime.Format(v["date"]))
		}
	} else {
		p.Logger.Errorf("replaceWebDate 数据转换出错：%v", err)
	}
	return bodyStr
}

func replaceWebImage(p *proxyServer, webImage string, bodyStr string) string {
	type randomImage struct {
		Position string   `json:"position"`
		Image    []string `json:"image"`
	}
	var replaceInfo map[string][]randomImage
	if err := json.Unmarshal([]byte(webImage), &replaceInfo); err == nil {
		for _, v := range replaceInfo["list"] {

			if find := strings.Contains(bodyStr, v.Position); find {
				re, _ := regexp.Compile(v.Position)
				var images string
				for _, i := range v.Image {
					images += "<img src=" + i + "> "
				}
				bodyStr = re.ReplaceAllString(bodyStr, v.Position+images)
			}
		}
	} else {
		p.Logger.Errorf("replaceWebImage 数据转换出错：%v", err)
	}
	return bodyStr
}

func replaceWebRandomHref(p *proxyServer, webRandomHref string, bodyStr string) string {
	var replaceInfo map[string][]map[string]string
	if err := json.Unmarshal([]byte(webRandomHref), &replaceInfo); err == nil {
		for _, v := range replaceInfo["list"] {
			if find := strings.Contains(bodyStr, v["position"]); find {
				re2 := regexp2.MustCompile(`\>(.*?)</a>`, 0)
				repl := ">" + v["prefix"] + "</a>"
				randomHref, err := re2.Replace(v["random_href"], repl, -1, -1)
				if err != nil {
					p.Logger.Error("随机链接替换出错", err)
				}
				re, _ := regexp.Compile(v["position"])
				bodyStr = re.ReplaceAllString(bodyStr, randomHref)
			}
		}
	} else {
		p.Logger.Errorf("replaceWebRandomHref 数据转换出错：%v", err)
	}
	return bodyStr
}

func replaceWebRandomContent(p *proxyServer, webRandomContent string, bodyStr string) string {
	type randomContent struct {
		Position      string   `json:"position"`
		RandomContent []string `json:"random_content"`
	}
	var replaceInfo map[string][]randomContent
	if err := json.Unmarshal([]byte(webRandomContent), &replaceInfo); err == nil {
		for _, v := range replaceInfo["list"] {
			if find := strings.Contains(bodyStr, v.Position); find {
				re, _ := regexp.Compile(v.Position)
				var content string
				for _, c := range v.RandomContent {
					content += c + "\n "
				}
				bodyStr = re.ReplaceAllString(bodyStr, content)
			}
		}
	} else {
		p.Logger.Errorf("replaceWebRandomContent 数据转换出错：%v", err)
	}
	return bodyStr
}
