package config

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
	"github.com/unknwon/goconfig"
	"go.uber.org/zap"
)

type Configs struct {
	Serve      serveConf
	Spider     []string
	GoodSpider []string
}

type serveConf struct {
	Host            string
	Port            string
	Ssl             bool
	ImageFolder     string
	Debug           bool
	JumpType        int
	JumpIndex       int
	SimulateSpiders int
	LogPath         string
	CachePath       string
	Dbname          string
	WebProxy        string
	SaveImages      bool
}

func NewConfig() *Configs {
	var config Configs
	viper.SetConfigName("/data/mirror")
	viper.SetConfigType("toml")
	viper.AddConfigPath(".")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			panic(fmt.Errorf("fatal error config file: %w", err))
		} else {
			panic(fmt.Errorf("fatal error config file: %w", err))
		}
	}
	viper.Set("charset", "utf8")
	viper.Set("maxopenconns", 20)
	viper.Set("maxidleconns", 10)
	viper.Set("MaxLifetimeConns", 7200)
	err := viper.Unmarshal(&config)
	config.Spider = getSpider()
	cfg := getEnvFile()
	config.Serve.ImageFolder = cfg.MustValue("serve", "IMAGE_FOLDER")
	config.Serve.Dbname = cfg.MustValue("", "DB_NAME")
	config.GoodSpider = getGoodSpider()
	config.Serve.LogPath = "./runtime/mirror/logs/"
	config.Serve.CachePath = "./runtime/mirror/cache/"
	if err != nil {
		return nil
	}
	viper.WatchConfig()
	viper.OnConfigChange(func(e fsnotify.Event) {
		// 配置文件发生变更之后会调用的回调函数
		err := viper.Unmarshal(&config)
		cfg := getEnvFile()
		config.Serve.ImageFolder = cfg.MustValue("serve", "IMAGE_FOLDER")
		config.Serve.Dbname = cfg.MustValue("", "DB_NAME")
		config.Spider = getSpider()
		config.GoodSpider = getGoodSpider()
		config.Serve.LogPath = "./runtime/mirror/logs/"
		config.Serve.CachePath = "./runtime/mirror/cache/"
		if err != nil {
			return
		}
	})
	return &config
}

func getSpider() []string {
	spider := []string{
		"TencentTraveler",
		"Baiduspider+",
		"Yisouspider",
		"BaiduGame",
		"Googlebot",
		"msnbot",
		"Sosospider+",
		"Sogou web spider",
		"ia_archiver",
		"Yahoo! Slurp",
		"YoudaoBot",
		"Yahoo Slurp",
		"MSNBot",
		"Java (Often spam bot)",
		"BaiDuSpider",
		"Voila",
		"Yandex bot",
		"BSpider",
		"twiceler",
		"Sogou Spider",
		"Speedy Spider",
		"Google AdSense",
		"Heritrix",
		"Python-urllib",
		"Alexa (IA Archiver)",
		"Ask",
		"Exabot",
		"Custo",
		"OutfoxBot/YodaoBot",
		"yacy",
		"SurveyBot",
		"legs",
		"lwp-trivial",
		"Nutch",
		"StackRambler",
		"The web archive (IA Archiver)",
		"Perl tool",
		"MJ12bot",
		"Netcraft",
		"MSIECrawler",
		"WGet tools",
		"larbin",
		"Fish search",
		"MauiBot",
		"MegaIndex",
		"DotBot",
		"AlphaBot",
		"MegaIndex",
		"AhrefsBot",
	}
	return spider
}

func getGoodSpider() []string {
	goodSpider := []string{
		"baiduspider",
		"Yisouspider",
		"360spider",
		"haosouspider",
		"sogou",
		"sosospider",
	}
	return goodSpider
}

func getEnvFile() *goconfig.ConfigFile {
	cfg, err := goconfig.LoadConfigFile(".env")
	if err != nil {
		zap.S().Fatalf("无法加载env配置文件：%s", err)
	}
	return cfg
}
