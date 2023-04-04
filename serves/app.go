package serves

import (
	"bufio"
	"github.com/farmerx/gorsa"
	"github.com/sgoby/opencc"
	"go.uber.org/zap"
	"golang.org/x/net/context"
	"net/http"
	"os"
	"seo_mirror/config"
	"seo_mirror/models"
	"sort"
	"strconv"
	"strings"
	"time"
)

type AppServe struct {
	Conf           *config.Configs
	server         *http.Server
	Db             *models.WebDb
	S2T            *opencc.OpenCC
	expireDate     string
	Logger         *zap.SugaredLogger
	siteConfigData map[string]models.WebSiteConfig
	queueData      *models.Queue
	spider         []string
}

func (a *AppServe) Start() bool {
	webSiteConfigs, err := a.Db.GetAll()
	if err != nil {
		a.Logger.Error("查询全部数据时出现错误", err.Error())
		return false
	}
	a.queueData = models.NewQueue()
	if len(webSiteConfigs) >= 1 {
		a.siteConfigData = make(map[string]models.WebSiteConfig)
		for _, siteConfig := range webSiteConfigs {
			if *siteConfig.Domain != "" {
				a.siteConfigData[*siteConfig.Domain] = siteConfig
			} else {
				a.queueData.Push(siteConfig)
			}
		}
	}
	a.getSpider()
	proxy := newProxyServer(&proxyServerOptions{
		useSSL:        a.Conf.Serve.Ssl,
		reqHeaders:    http.Header{},
		resHeaders:    http.Header{},
		proxyExternal: true,
		cors:          false,
		noCache:       true,
		cacheFolder:   a.Conf.Serve.CachePath,
	}, a)
	http.HandleFunc("/", proxy.Handler())
	http.HandleFunc(a.Conf.Serve.ImageFolder, proxy.showPicHandle)
	http.HandleFunc("/notice/api", a.notice)
	a.server = &http.Server{
		Addr:         a.Conf.Serve.Host + ":" + a.Conf.Serve.Port,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	a.server.SetKeepAlivesEnabled(false)
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.Logger.Error("监听出错: %a\n", err)
		}
	}()
	//c := cron.New(cron.WithSeconds())
	//c.AddFunc("@hourly", func() { a.resetData() })
	//c.AddFunc("*/30 * * * * *", func() { a.resetData() })
	//c.Start()
	return true
}

func (a *AppServe) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.server.Shutdown(ctx); err != nil {
		a.Logger.Fatal("服务器关闭：", err)
	}

	select {
	case <-ctx.Done():
		a.Logger.Infof("超时5秒")
	}
}

func (a *AppServe) ResetData() {
	webSiteConfigs, err := a.Db.GetAll()
	if err != nil {
		a.Logger.Error("更新数据", err.Error())
	}
	if len(webSiteConfigs) >= 1 {
		a.siteConfigData = make(map[string]models.WebSiteConfig)
		a.queueData.Clear()
		for _, siteConfig := range webSiteConfigs {
			a.Logger.Infof("更新队列数据:%+v\n", *siteConfig.Domain)
			if *siteConfig.Domain != "" {
				a.siteConfigData[*siteConfig.Domain] = siteConfig
			} else {
				a.queueData.Push(siteConfig)
			}
		}
	}
}

func (a *AppServe) notice(w http.ResponseWriter, r *http.Request) {
	webSiteConfigs, err := a.Db.GetAll()
	if err != nil {
		a.Logger.Error("更新数据", err.Error())
	}
	if len(webSiteConfigs) >= 1 {
		a.siteConfigData = make(map[string]models.WebSiteConfig)
		a.queueData.Clear()
		for _, siteConfig := range webSiteConfigs {
			if *siteConfig.Domain != "" {
				a.siteConfigData[strings.TrimSpace(*siteConfig.Domain)] = siteConfig
			} else {
				a.queueData.Push(siteConfig)
			}
		}
	}
	a.Logger.Info("数据更新完成！！！")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("{\"code\":200}"))
}

func (a *AppServe) GetUseTime() (string, error) {
	pubKey := `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0cJWuX8TX4xvj4bwRnp+
ieUExTdvRRThKiff9wYFqZiIMOOppmiSAfGDJ+kFnDDlSXqRXlfViZudQUSYCW5w
ruLEuhx9MuaFkhaKZUVhGpsM25KU7LyzyJxFOfK2f0K1YJFZz+GN5GsPGcoscdIi
5SLDI7frBnWupSlSpBbsAQzyaV+jT2Oil5DNKxLUr+hVcPBusIV6ktt5gDRv6AmI
2mWs0FHI2CJjn9j3edXgTI57wLuTFmUaLaPPJWFMwSQ3ZpbbTd3NVkxHtNY4FPCc
ddrzWA0+GOj5kKcjdNWnzFsQBLjOa2FXymhMa2+UBA26B3rhyP4anRTm5jDpskgr
JwIDAQAB
-----END PUBLIC KEY-----
`
	if err := gorsa.RSA.SetPublicKey(pubKey); err == nil {
		authContent, err := os.ReadFile("data/auth.cert")
		decrypt, err := gorsa.RSA.PubKeyDECRYPT(authContent)
		if err != nil {
			return "", err
		}
		return string(decrypt), nil
	} else {
		a.Logger.Error("解码错误")
	}
	return "", nil
}

func (a *AppServe) Auth() bool {
	expireDate, _ := a.GetUseTime()
	expire, err := time.ParseInLocation("2006-01-02 15:04:05", expireDate, time.Local)
	a.Logger.Info("服务到期时间：", expireDate)
	if err != nil || !expire.After(time.Now()) {
		return true
	}
	return false
}

func (a *AppServe) getSpider() {
	// open file
	for i := 1; i <= 8; i++ {
		f, err := os.Open("./data/spider/" + strconv.Itoa(i) + ".json")
		if err != nil {
			a.Logger.Fatal(err)
		}
		// 使用扫描仪逐行读取文件
		scanner := bufio.NewScanner(f)

		for scanner.Scan() {
			a.spider = append(a.spider, scanner.Text())
		}
		f.Close()
		if err := scanner.Err(); err != nil {
			a.Logger.Fatal(err)
		}
	}
	sort.Strings(a.spider)
}
