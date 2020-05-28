package main

import (
	"context"
	"fmt"
	"github.com/chromedp/chromedp"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gocolly/colly"
	"github.com/gocolly/colly/extensions"
	"github.com/gocolly/redisstorage"
	_ "github.com/gomodule/redigo/redis"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)


var db *gorm.DB

func init() {
	var err error
	db, err = gorm.Open("mysql", "root:123456@tcp(127.0.0.1:3306)/douyu?charset=utf8&parseTime=True&loc=Local")
	if err != nil {
		panic(err)
	}
}


type Anchor struct {
	AnchorId string
	Name    string
	RoomUrl      string
	Fontfile      string
	Fans  int
	Hot  int
	Level int
	Zone string
}

func main() {
	//anchors := make([]Anchor, 0, 20000)
	host := "www.douyu.com"
	c := colly.NewCollector(
		colly.AllowedDomains(host),
		//colly.CacheDir("./dayu_cache"),
	)

	extensions.RandomUserAgent(c)
	extensions.Referer(c)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*"+ host +"*",
		Parallelism: 10,

	})

	detail := c.Clone()

	// create the redis storage
	storage := &redisstorage.Storage{
		Address:  "127.0.0.1:6379",
		Password: "",
		DB:       0,
		Prefix:   "httpbin_test",
	}

	// add storage to the collector
	err := c.SetStorage(storage)
	if err != nil {
		panic(err)
	}

	// delete previous data from storage
	if err := storage.Clear(); err != nil {
		log.Fatal(err)
	}

	// Before making a request print "Visiting ..."
	c.OnRequest(func(r *colly.Request) {
		fmt.Println("C Visiting", r.URL.String())
	})

	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request URL:", r.Request.URL, "failed with response:", r, "\nError:", err)
	})

	// On every a element which has href attribute call callback
	c.OnHTML("div.ListContent>ul>li>div>a", func(e *colly.HTMLElement) {
		href := e.Attr("href")
		roomUrl := "https://" + host + href
		log.Println("Found room: ", roomUrl)
		courseURL := e.Request.AbsoluteURL(roomUrl)
		if strings.Index(courseURL, "douyu.com") != -1 {
			detail.Visit(courseURL)
		}
	})

	detail.OnRequest(func(r *colly.Request) {
		log.Println("D Visiting", r.URL.String())
	})

	detail.OnHTML("div.Title-roomInfo", func(e *colly.HTMLElement) {
		var roomUrl string
		roomUrl = e.Request.URL.Scheme + "://" + e.Request.URL.Host  + e.Request.URL.Path
		getdetail(roomUrl)
	})

	c.Visit("https://www.douyu.com/directory/all")
	c.Wait()
}


func getdetail(url string)  {
	// create context
	ctx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	//ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
	//defer cancel()

	// run task list
	var fanshtml string
	var levelhtml string
	var fans string
	var hot string
	var name string
	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`.Title-followBtnBox>.Title-followNum>i`),
		chromedp.Sleep(5 * time.Second),
		chromedp.OuterHTML(`.Title-followNum`, &fanshtml, chromedp.ByQuery), //获
		chromedp.TextContent(`.Title-followNum`, &fans, chromedp.ByQuery),
		chromedp.TextContent(`.Title-anchorText`, &hot, chromedp.ByQuery),
		chromedp.TextContent(`.Title-anchorNameH2`, &name, chromedp.ByQuery),
		chromedp.OuterHTML(`.Title-AnchorLevel`, &levelhtml, chromedp.ByQuery),
	)
	if err != nil {
		log.Fatal(err)
	}

	//下载字体
	r,_ := regexp.Compile(`style="font-family: douyu(.*);"`)
	str := r.FindStringSubmatch(fanshtml)
	var woffname string
	woffname = str[1]
	woff := "https://shark.douyucdn.cn/app/douyu/res/font/" + woffname + ".woff"
	res, err := http.Get(woff)
	if err != nil {
		panic(err)
	}
	f, err := os.Create("./douyufront/" + woffname + ".woff")
	if err != nil {
		panic(err)
	}
	io.Copy(f, res.Body)


	rs,_ := regexp.Compile(`https://www.douyu.com/(.*)`)
	math := rs.FindStringSubmatch(url)
	hots,_ := strconv.Atoi(hot)
	fansint,_ := strconv.Atoi(fans)

	//匹配等级
	rr,_ := regexp.Compile(`AnchorLevel AnchorLevel-(\d*)`)
	str2 := rr.FindStringSubmatch(levelhtml)
	levelint,_ := strconv.Atoi(str2[2])

	//保存到数据
	anchor := &Anchor{
		AnchorId:math[1],
		Name:name,
		RoomUrl:url,
		Hot:hots,
		Fontfile:woff,
		Fans:fansint,
		Level:levelint,
	}
	if err := db.Create(anchor).Error; err != nil {
		return
	}

}

