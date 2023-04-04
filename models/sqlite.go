package models

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"strings"
)

type WebSiteConfig struct {
	Id               int
	Domain           *string
	OrgUrl           string
	WebTitle         string
	WebSeoTitle      string
	WebKeywords      string
	WebDescription   string
	ContentTitle     *string
	WebFriendLink    *string
	WebRandomHref    *string
	WebRandomContent *string
	WebDate          *string
	WebImage         *string
	WebReplaces      string
	WebJs            int8
	WebS2t           int8
	WebCacheEnable   int8
	WebH1replace     *string
	WebCacheTime     int64
	cTime            string
	Linkup           int8
	Isdel            int8
}

type WebDb struct {
	db     *sql.DB
	dbname string
}

func NewDb(dbname string) (*WebDb, error) {
	web := &WebDb{}
	db, err := sql.Open("sqlite3", dbname)
	if err != nil {
		return nil, err
	}
	web.db = db
	web.dbname = dbname
	return web, nil
}
func (w *WebDb) GetOne(domain string) (WebSiteConfig, error) {
	domain = strings.TrimSpace(domain)
	var web WebSiteConfig
	rs, err := w.db.Query("select * from website_config where domain=?", domain)
	if err != nil {
		return web, err
	}

	if rs.Next() {
		err = rs.Scan(
			&web.Id,
			&web.Domain,
			&web.OrgUrl,
			&web.WebTitle,
			&web.WebSeoTitle,
			&web.WebKeywords,
			&web.WebDescription,
			&web.ContentTitle,
			&web.WebFriendLink,
			&web.WebRandomHref,
			&web.WebRandomContent,
			&web.WebDate,
			&web.WebImage,
			&web.WebReplaces,
			&web.WebJs,
			&web.WebS2t,
			&web.WebCacheEnable,
			&web.WebH1replace,
			&web.WebCacheTime,
			&web.cTime,
			&web.Linkup,
			&web.Isdel,
		)
		if err != nil {
			return web, err
		}

	}
	err = rs.Close()
	if err != nil {
		return web, err
	}
	if web.Id == 0 {
		return web, errors.New("无搜索结果")
	}
	return web, nil

}
func (w *WebDb) DeleteOne(id int) error {
	_, err := w.db.Exec("delete from website_config where id=?", id)
	if err != nil {
		return err
	}
	return nil
}
func (w *WebDb) GetAll() ([]WebSiteConfig, error) {
	rs, err := w.db.Query("select * from website_config where isdel=0")
	if err != nil {
		return nil, err
	}
	var results = make([]WebSiteConfig, 0)
	for rs.Next() {
		var web WebSiteConfig
		err := rs.Scan(
			&web.Id,
			&web.Domain,
			&web.OrgUrl,
			&web.WebTitle,
			&web.WebSeoTitle,
			&web.WebKeywords,
			&web.WebDescription,
			&web.ContentTitle,
			&web.WebFriendLink,
			&web.WebRandomHref,
			&web.WebRandomContent,
			&web.WebDate,
			&web.WebImage,
			&web.WebReplaces,
			&web.WebJs,
			&web.WebS2t,
			&web.WebCacheEnable,
			&web.WebH1replace,
			&web.WebCacheTime,
			&web.cTime,
			&web.Linkup,
			&web.Isdel,
		)
		if err != nil {
			return nil, err
		}

		results = append(results, web)
	}
	_ = rs.Close()

	return results, nil
}

func (w *WebDb) UpdateById(data WebSiteConfig) error {
	db, err := sql.Open("sqlite3", w.dbname)
	defer db.Close()
	if err == nil {
		updateSql := "update website_config set domain=? where id=?"
		fmt.Println(updateSql)
		offSyn := "PRAGMA synchronous = OFF"
		sqlMod := "PRAGMA journal_mode=WAL"
		_, err = db.Exec(offSyn)
		_, err = db.Exec(sqlMod)
		_, err = db.Exec(updateSql, data.Domain, data.Id)
		if err != nil {
			return err
		}
	}
	return nil
}
func (w *WebDb) UpdateFriendLink(data WebSiteConfig) error {
	db, err := sql.Open("sqlite3", w.dbname)
	defer db.Close()
	if err == nil {
		updateSql := "update website_config set web_friend_link=?,linkup=1 where id=?"
		fmt.Println(updateSql)
		offSyn := "PRAGMA synchronous = OFF"
		sqlMod := "PRAGMA journal_mode=WAL"
		_, err := w.db.Exec(offSyn)
		_, err = w.db.Exec(sqlMod)
		_, err = w.db.Exec(updateSql, data.WebFriendLink, data.Id)
		if err != nil {
			return err
		}
	}
	return nil

}
func (w *WebDb) GetByPage(page, limit int) ([]WebSiteConfig, error) {
	start := (page - 1) * limit
	querySql := fmt.Sprintf("select * from website_config limit %d,%d", start, limit)
	rs, err := w.db.Query(querySql)
	if err != nil {
		return nil, err
	}
	var results = make([]WebSiteConfig, 0)
	for rs.Next() {
		var web WebSiteConfig

		err := rs.Scan(&web)
		if err != nil {
			return nil, err
		}

		results = append(results, web)
	}
	_ = rs.Close()
	return results, nil
}

func (w *WebDb) MultiDel(domains []string) error {
	args := make([]interface{}, len(domains))
	for i, id := range domains {
		args[i] = id
	}
	delSql := `delete from website_config where domain in (?` + strings.Repeat(",?", len(args)-1) + `)`
	_, err := w.db.Exec(delSql, args...)
	if err != nil {
		return err
	}
	return nil

}

func (w *WebDb) Count() (int, error) {
	countSql := `select count(*) as count from website_config`
	rs, err := w.db.Query(countSql)
	if err != nil {
		return 0, err
	}
	var count int
	rs.Next()
	err = rs.Scan(&count)
	if err != nil {
		return 0, err
	}
	err = rs.Close()
	if err != nil {
		return 0, err
	}
	return count, nil

}
func (w *WebDb) ForbiddenWordReplace(forbiddenWord, replaceWord, splitWord string) ([]string, error) {
	forbiddenSql := "select domain,index_title from website_config where index_title like ?"
	rs, err := w.db.Query(forbiddenSql, "%"+forbiddenWord+"%")
	if err != nil {
		return nil, err
	}
	var indexTitleArr = make(map[string]string)
	var temp string
	var tempDomain string
	for rs.Next() {
		err = rs.Scan(&tempDomain)
		if err != nil {
			return nil, err
		}
		err = rs.Scan(&temp)
		if err != nil {
			return nil, err
		}
		indexTitleArr[tempDomain] = temp
	}
	_ = rs.Close()
	if len(indexTitleArr) == 0 {
		return nil, errors.New("没有找到要替换的禁词")
	}
	var domainArr = make([]string, 0)
	updateSql := `update website_config set index_title=? where index_title=?`
	for domain, title := range indexTitleArr {
		if strings.Contains(title, forbiddenWord+splitWord) || strings.Contains(title, splitWord+forbiddenWord) {
			words := strings.Split(title, splitWord)
			for i, word := range words {
				if word == forbiddenWord {
					words[i] = replaceWord
				}
			}
			newTitle := strings.Join(words, splitWord)
			_, err := w.db.Exec(updateSql, newTitle, title)
			if err != nil {
				return nil, err
			}
			dn := domain + "##" + newTitle
			domainArr = append(domainArr, dn)
		}
	}
	return domainArr, err
}

func (w *WebDb) InitTable() error {
	rs, err := w.db.Query(`SELECT count(*) as count FROM sqlite_master WHERE type='table' AND name = 'website_config'`)
	if err != nil {
		return err
	}
	var count int
	rs.Next()
	rs.Scan(&count)
	rs.Close()
	if count == 0 {
		_, err = w.db.Exec(`CREATE TABLE "website_config" (
  "id" integer PRIMARY KEY AUTOINCREMENT,
  "domain" varchar(30) NOT NULL, --- 生成后的网站二级域名,不允许重复生成
  "org_url" TEXT NOT NULL, --- 模板来源网站url，源站url
  "web_title" TEXT NOT NULL, --- 网站标题
  "web_keywords" TEXT, --- 网站关键词
  "web_description" TEXT, --- 网站描述
  "content_title" TEXT, --- 内容页标题
  "web_friend_link" TEXT, --- 网站友情链接
  "web_random_href" TEXT, --- 网站随机url
  "web_random_content" TEXT, --- 网站随机内容
  "web_date" TEXT, --- 日期
  "web_image" varchar(100), --- 网站图片
  "web_replaces" TEXT, --- 关键词替换信息
  "web_js" integer DEFAULT 1, --- 是否开启缓存JS
  "web_s2t" integer DEFAULT 0, --- 是否开启简体转繁体
  "web_cache_enable" integer DEFAULT 1, --- 是否开启缓存
  "web_h1replace" TEXT, --- H1标题
  "web_cache_time" integer, --- 缓存失效时间
  "c_time" integer NOT NULL, --- 网站创建时间
  UNIQUE ("domain" ASC)
);
 
CREATE UNIQUE INDEX "index_website_config_domain_1"
ON "website_config" (
  "domain" ASC
);`)

	}
	return err
}
