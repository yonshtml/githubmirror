package main

import (
	"github.com/sgoby/opencc"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"seo_mirror/config"
	"seo_mirror/models"
	"seo_mirror/serves"
)

func main() {
	conf := config.NewConfig()
	zap.ReplaceGlobals(serves.Logger(conf))
	logger := zap.S()
	if conf.Serve.ImageFolder == "" {
		logger.Error("请在env里面添加图片路径")
		return
	}
	db, err := models.NewDb(conf.Serve.Dbname)
	if err != nil {
		logger.Error("连接数据库错误", err.Error())
		return
	}

	s2t, err := opencc.NewOpenCC("s2t")
	if err != nil {
		logger.Error("转繁体功能错误", err.Error())
		return
	}
	app := serves.AppServe{
		Conf:   conf,
		Db:     db,
		S2T:    s2t,
		Logger: logger,
	}
	if app.Auth() {
		logger.Error("服务已到期，请找管理员续期")
		return
	}
	if app.Start() {
		logger.Infof("服务启动成功...")
		quit := make(chan os.Signal)
		signal.Notify(quit, os.Interrupt)
		<-quit
		app.Stop()
		logger.Error("服务器退出")
	} else {
		logger.Warn("服务没有启动成功...")
	}
}
