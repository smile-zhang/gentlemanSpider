package honcrawler

import (
	"fmt"
	"sync"
	"time"

	"github.com/Youngkingman/gentlemanSpider/settings"
	colly "github.com/gocolly/colly/v2"
)

/*
Some constant for the spider
*/
const Host = `https://www.wnacg.com`                                                                        // 定义常量Host，表示目标网站的主机地址
const GallaryUrl string = Host + `/albums-index-page-%d.html`                                               // 定义常量GallaryUrl，表示相册页面的URL模板
const UserAgent string = `Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:104.0) Gecko/20100101 Firefox/104.0` // 定义常量UserAgent，表示HTTP请求的用户代理
const ImgsPerPage int = 12                                                                                  // 定义常量ImgsPerPage，表示每页的图片数量

/*
	The coordinator manager all the concurrent behaviors.
*/

type coordinator struct {
	tagChannel   chan string     // 定义一个字符串通道，用于传递标签
	honChannel   chan *HonDetail // 定义一个指向HonDetail的通道，用于传递HonDetail数据
	limitChannel chan struct{}   // 定义一个空结构体通道，用于限制并发数量

	gWaitGroup sync.WaitGroup // 定义一个WaitGroup，用于等待生成任务完成
	dWaitGroup sync.WaitGroup // 定义一个WaitGroup，用于等待消费任务完成

	tagSet set        // 定义一个set类型，用于存储标签
	mutex  sync.Mutex // 定义一个互斥锁，用于保护tagSet的并发访问
}

var Coordinator = coordinator{
	honChannel:   make(chan *HonDetail, settings.CrawlerSetting.HonBuffer),        // 初始化honChannel通道，缓冲区大小为HonBuffer
	tagChannel:   make(chan string, settings.CrawlerSetting.TagBuffer),            // 初始化tagChannel通道，缓冲区大小为TagBuffer
	limitChannel: make(chan struct{}, settings.CrawlerSetting.HonConsumerCount/2), // 初始化limitChannel通道，缓冲区大小为HonConsumerCount的一半

	gWaitGroup: sync.WaitGroup{}, // 初始化gWaitGroup
	dWaitGroup: sync.WaitGroup{}, // 初始化dWaitGroup

	tagSet: make(map[string]struct{}), // 初始化tagSet为一个空的map
}

// base collector
var collector = colly.NewCollector(
	colly.UserAgent(UserAgent), // 设置UserAgent
	colly.AllowURLRevisit(),    // 允许URL重新访问
)

func init() {
	collector.SetRequestTimeout(120 * time.Second) // 设置请求超时时间为120秒
	// Error Handler
	collector.OnError(func(r *colly.Response, err error) {
		fmt.Printf("Error %s: %v\n", r.Request.URL, err) // 定义错误处理函数，打印错误信息
	})
	// proxy setting
	if settings.CrawlerSetting.EnableProxy {
		collector.SetProxy(settings.CrawlerSetting.ProxyHost) // 如果启用了代理，设置代理地址
	}
}

func (c *coordinator) sendHon(hd *HonDetail) {
	c.honChannel <- hd // 将HonDetail数据发送到honChannel通道
}

func (c *coordinator) sendTag(tag string) {
	c.tagChannel <- tag // 将标签发送到tagChannel通道
}

func (c *coordinator) generateHon(pSt int, pEnd int) {
	for i := pSt; i <= pEnd; i++ { // 遍历从pSt到pEnd的所有页码
		c.limitChannel <- struct{}{} // 向limitChannel通道发送一个空结构体，限制并发数量
		c.gWaitGroup.Add(1)          // 增加gWaitGroup的计数
		go func(i int) {             // 启动一个新的goroutine
			infos := GenGallaryInfos(i)  // 生成相册信息
			for _, info := range infos { // 遍历相册信息
				d := GenHonDetails(info) // 生成HonDetail数据
				if d.PageNum > 500 {     // 如果页面数量大于500，跳过
					continue
				}
				if !parseTages(d.Tags) { // 如果标签解析失败，跳过
					continue
				}
				if settings.CrawlerSetting.TagConsumerCount > 0 { // 如果启用了标签消费者
					for _, t := range d.Tags { // 遍历标签
						c.sendTag(t) // 发送标签
					}
				}
				c.sendHon(d)     // 发送HonDetail数据
				<-c.limitChannel // 从limitChannel通道接收一个空结构体，释放并发限制
			}
			c.gWaitGroup.Done() // 减少gWaitGroup的计数
		}(i)
	}
}

func (c *coordinator) consumeHon(cnt int) {
	for i := 0; i < cnt; i++ { // 启动cnt个goroutine
		c.dWaitGroup.Add(1) // 增加dWaitGroup的计数
		go func() {         // 启动一个新的goroutine
			for hon := range c.honChannel { // 从honChannel通道接收HonDetail数据
				Download(hon) // 下载HonDetail数据
			}
			c.dWaitGroup.Done() // 减少dWaitGroup的计数
		}()
	}
}

func (c *coordinator) consumeTag(cnt int) {
	for i := 0; i < cnt; i++ { // 启动cnt个goroutine
		c.dWaitGroup.Add(1) // 增加dWaitGroup的计数
		go func() {         // 启动一个新的goroutine
			for tag := range c.tagChannel { // 从tagChannel通道接收标签
				c.mutex.Lock()          // 加锁
				if !c.tagSet.has(tag) { // 如果tagSet中没有该标签
					c.tagSet.insert(tag) // 插入标签到tagSet
					SaveTag(tag)         // 保存标签
				}
				c.mutex.Unlock() // 解锁
			}
			c.dWaitGroup.Done() // 减少dWaitGroup的计数
		}()
	}
}

func (c *coordinator) Start() {
	c.consumeHon(settings.CrawlerSetting.HonConsumerCount) // 启动HonDetail消费者
	if settings.CrawlerSetting.TagConsumerCount > 0 {      // 如果启用了标签消费者
		c.consumeTag(settings.CrawlerSetting.TagConsumerCount) // 启动标签消费者
	}
	c.generateHon(
		settings.CrawlerSetting.PageStart,
		settings.CrawlerSetting.PageEnd,
	) // 生成HonDetail数据

	c.gWaitGroup.Wait() // 等待所有生成任务完成
	close(c.honChannel) // 关闭honChannel通道
	close(c.tagChannel) // 关闭tagChannel通道
	c.dWaitGroup.Wait() // 等待所有消费任务完成
}
