package honcrawler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gocolly/colly/v2"
)

/*
	This `go` file will generate the details of ero-hons and collect
	there tags.
	这个Go文件将生成ero-hons的详细信息并收集它们的标签。
*/

// from https://www.wnacg.com/albums-index-page-1.html
// to https://www.wnacg.com/albums-index-page-7101.html

// page number regex
var patternNum = regexp.MustCompile(`(\d+)P`) // 定义一个正则表达式，用于匹配页面数量

// typical HonUrl /photos-index-aid-169728.html
type GallaryInfo struct {
	HonUrl string // 本子页面的URL
	Title  string // 本子的标题
}

type HonDetail struct {
	Tags    []string // 本子的标签
	Title   string   // 本子的标题
	PageNum int      // 本子的页面数量，可以计算翻页次数
	Images  []string // 按顺序的本子页面URL
}

// GenGallaryInfos 函数生成指定页面的相册信息
func GenGallaryInfos(page int) (infos []*GallaryInfo) {
	GallaryCollector := collector.Clone() // 克隆一个新的collector实例
	GallaryCollector.OnHTML(".pic_box>a", func(e *colly.HTMLElement) {
		fmt.Print("GenGallaryInfos 开始回调")
		// 当匹配到.pic_box>a元素时，执行此回调函数
		info := &GallaryInfo{}
		info.HonUrl = e.Attr("href") // 获取href属性，赋值给HonUrl
		info.Title = e.Attr("title") // 获取title属性，赋值给Title
		infos = append(infos, info)  // 将info添加到infos切片中
	})

	GallaryCollector.OnRequest(func(r *colly.Request) {
		// 当发起请求时，执行此回调函数
		fmt.Println("Requesting:", r.URL) // 打印请求的URL
	})

	// 设置 OnResponse 回调函数
	GallaryCollector.OnResponse(func(r *colly.Response) {
		fmt.Println("Received response from:", r.Request.URL)
	})
	// 设置 OnError 回调函数
	GallaryCollector.OnError(func(r *colly.Response, err error) {
		fmt.Printf("Error: %s, %v\n", r.Request.URL, err)
	})

	url := fmt.Sprintf(GallaryUrl, page) // 格式化URL，替换占位符为page
	GallaryCollector.Visit(url)          // 访问生成的URL
	return
}

// GenHonDetails 函数生成指定相册信息的详细信息
func GenHonDetails(g *GallaryInfo) (details *HonDetail) {
	details = &HonDetail{
		Tags:   make([]string, 0), // 初始化Tags切片
		Title:  g.Title,           // 设置Title为相册信息的Title
		Images: make([]string, 0), // 初始化Images切片
	}

	details.crawlTagAndPage(g) // 爬取标签和页面数量
	details.crawlImages(g)     // 爬取图片URL

	return
}

// crawlTagAndPage 方法爬取标签和页面数量
func (hd *HonDetail) crawlTagAndPage(g *GallaryInfo) {
	HonCollector := collector.Clone() // 克隆一个新的collector实例
	// 对于标题信息的处理，获取Tags和PageNum
	HonCollector.OnHTML(".uwconn", func(e *colly.HTMLElement) {
		e.ForEach(".uwconn>label", func(i int, h *colly.HTMLElement) {
			// 遍历.uwconn>label元素
			if i == 0 { // 分类，解析到tag里，从0开始
				tags := strings.Split(h.Text, " / ") // 按" / "分割标签字符串
				hd.Tags = append(hd.Tags, tags...)   // 将标签添加到Tags切片中
			}
			if i == 1 { // 页数，解析到PageNum
				pageStr := patternNum.FindAllStringSubmatch(h.Text, -1) // 使用正则表达式匹配页面数量
				cnt, err := strconv.Atoi(pageStr[0][1])                 // 将匹配到的字符串转换为整数
				if err != nil {
					fmt.Printf("wrong with str unmarshal %v", pageStr) // 打印错误信息
				}
				hd.PageNum = cnt // 设置PageNum为匹配到的页面数量
			}
		})
		e.ForEach(".tagshow", func(_ int, h *colly.HTMLElement) {
			// 遍历.tagshow元素
			hd.Tags = append(hd.Tags, h.Text) // 将标签文本添加到Tags切片中
		})
	})
	HonCollector.Visit(Host + g.HonUrl) // 访问本子页面的URL
}

// crawlImages 方法爬取图片URL
func (hd *HonDetail) crawlImages(g *GallaryInfo) {
	HonCollector := collector.Clone()   // 克隆一个新的collector实例
	total := hd.PageNum/ImgsPerPage + 1 // 计算总页数
	HonCollector.OnHTML(".pic_box>a", func(e *colly.HTMLElement) {
		// 当匹配到.pic_box>a元素时，执行此回调函数
		hd.Images = append(hd.Images, e.Attr("href")) // 将图片URL添加到Images切片中
	})

	for i := 1; i <= total; i++ { // 遍历所有页码
		url := pageUrlTrans(g.HonUrl, i) // 生成每一页的URL
		HonCollector.Visit(Host + url)   // 访问生成的URL
	}
}
