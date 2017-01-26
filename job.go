package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/fnv"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/jinzhu/gorm"
	_ "github.com/lib/pq"
)

var postgresConnString string = "postgresql://" + os.Getenv("POSTGRES_USER") + ":" + os.Getenv("POSTGRES_PASSWORD") + "@" + os.Getenv("POSTGRES_PORT_5432_TCP_ADDR") + "/" + os.Getenv("POSTGRES_DB")

var imageServer = os.Getenv("IMAGE_SERVER")

var popularFeedAddr = "http://xxx/api/feeds/popular"

const (
	root                = "http://www.mangareader.net"
	accountName         = "xxx"
	accountKey          = "xxx"
	containerAccessType = "blob"
)

type Category struct {
	Name string
	Link *url.URL
}

type Chapter struct {
	Name string
	Link *url.URL
}

type ChapterJobContext struct {
	Category Category
	Chapter  Chapter
	Pages    []Page
}

type Page struct {
	PageNo        int
	Link          *url.URL
	MangaImageSrc *url.URL
}

type PageWorkerResult struct {
	Val DbPage
	Err error
}

type DbCategory struct {
	ID                  int
	Name                string `sql:"size:512"`
	CategoryImage       string `sql:"size:512"`
	HostedCategoryImage string `sql:"size:10120"`
	Genres              []DbGenre
	AltName             string `sql:"size:512"`
	YearOfRelease       string `sql:"size:512"`
	Status              string `sql:"size:512"`
	Author              string `sql:"size:512"`
	Artist              string `sql:"size:512"`
	Description         string `sql:"size:10120"`
	Link                string `sql:"size:512"`
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type DbGenre struct {
	ID           int
	Name         string
	DbCategory   DbCategory
	DbCategoryID int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DbChapter struct {
	ID           int
	Name         string `sql:"size:10120"`
	Link         string `sql:"size:512"`
	ChapterNo    int
	TotalPages   int
	ScrappedTime int64
	Pages        []DbPage
	DbCategory   DbCategory
	DbCategoryID int
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type DbPage struct {
	ID             int
	MangaSrc       string `sql:"size:10512"`
	HostedMangaSrc string `sql:"size:10512"`
	PageNo         int
	DbChapter      DbChapter
	DbChapterID    int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DbHit struct {
	ID          int
	Count       int
	ChapterName string `sql:"size:10512"`
}

type DbCategoryProcessing struct {
	ID           int
	CategoryName string `sql:"size:10512"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type CategoryFromFeedServer struct {
	CategoryName string `json:"manga_name"`
}

var https chan http.Client

func acquire() (c http.Client) {
	c = <-https
	return
}

func release(c http.Client) {
	https <- c
}

var db *gorm.DB

func init() {

	log.Println("running")
	if imageServer == "" {
		panic("ENV VAR IMAGE_SERVER NOT SET")
	}

	initHttpClients()

	log.Println(postgresConnString)

	gormDb, err := gorm.Open("postgres", postgresConnString)

	if err != nil {
		log.Fatal(err)
	}

	db = &gormDb

	db.DB().Ping()

	db.DB().SetMaxIdleConns(-1)
	db.DB().SetMaxOpenConns(-1)

	db.SingularTable(true)

	rand.Seed(time.Now().UnixNano())
}

func unmount() {
	exec.Command("umount", []string{"./images"}...)
}

func mount() {
	var cmdOut []byte
	var err error

	if cmdOut, err = exec.Command("sshfs", []string{"root@xxx.xxx.xxx.xxx/images", "./images", "-o", "nonempty"}...).Output(); err != nil {
		panic("unable to mount")
	}

	log.Println(string(cmdOut))
}

func initHttpClients() {

	httpTimeout := 2 * time.Minute

	slice := make([]http.Client, 0)

	slice = append(slice, http.Client{Timeout: httpTimeout})

	log.Printf("there are %v available http clients for use \n", len(slice))

	https = make(chan http.Client, len(slice))

	for _, c := range slice {
		https <- c
	}
}

func createBucketFolder() {

	for i := 0; i < 100; i++ {
		path := fmt.Sprintf("images/%v", i)
		os.MkdirAll(path, 0777)
	}
}

func main() {

	runModePtr := flag.String("runMode", "full", "run mode: either 'full' or 'top30' only")
	isReversePtr := flag.Bool("isReverse", false, "run reverse?")

	flag.Parse()

	log.Println("runMode:", *runModePtr)
	log.Println("isReverse:", *isReversePtr)

	for {
		log.Println("running job")

		categories, err := getCategoriesFromSite()

		if err != nil {
			time.Sleep(5 * time.Minute)
			continue
		}

		if *runModePtr == "top30" {
			categories = filterToTop30(categories)
		}

		fmt.Printf("number of categories to process %v\n", len(categories))

		if err != nil {
			time.Sleep(5 * time.Minute)
			continue
		}

		var isReverse = *isReversePtr

		if isReverse {
			log.Println("running in reverse")
			categories = reverse(categories)
		}

		for _, category := range categories {

			cat := category

			// check if category was already processing, if it is, go to next loop.
			dbCategoryProcessing := &DbCategoryProcessing{}
			queryCategoryName := ReplaceSpecial(cat.Name)
			db.Where(&DbCategoryProcessing{CategoryName: queryCategoryName}).First(dbCategoryProcessing)

			if dbCategoryProcessing.CategoryName == queryCategoryName {
				log.Println("category " + queryCategoryName + " is in processing state")
				time.Sleep(3 * time.Second)
				continue
			}

			log.Println("picked " + cat.Name + " to process")

			// set category to be in processing state
			dbCategoryProcessing.CategoryName = queryCategoryName
			db.Create(dbCategoryProcessing)

			log.Println("have set " + cat.Name + " to processing")

			for _, job := range getNewJobs(cat) {
				job := job
				log.Println("new job received ", job.Chapter.Name)
				worker(job)
			}

			log.Println("completed processing category " + cat.Name)

			log.Println("unlocking")
			log.Println(dbCategoryProcessing)
			// unlock category
			db.Delete(dbCategoryProcessing)
		}
	}
}

func filterToTop30(categories []Category) (result []Category) {
	log.Println("geting top 30 only")

	res, err := http.Get(popularFeedAddr)
	if err != nil {
		log.Fatal(err)
	}

	body, err := ioutil.ReadAll(res.Body)
	defer res.Body.Close()

	if err != nil {
		log.Fatal(err)
	}

	var dat []CategoryFromFeedServer
	err = json.Unmarshal(body, &dat)
	if err != nil {
		log.Fatal(err)
	}

	for _, cat := range categories {
		if feedsContainCategory(dat, cat) {
			result = append(result, cat)
		}
	}

	return
}

func getJson(url string, target interface{}) error {
	r, err := http.Get(url)
	if err != nil {
		return err
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func worker(job ChapterJobContext) {

	dbCategory, err := getDbCategory(job.Category)

	if err != nil {
		log.Println(err)
		return
	}

	dbPages, err := processPages(job.Chapter)

	if err != nil {
		log.Println(err)
		return
	}

	chapterNo := strings.Replace(strings.TrimSpace(job.Chapter.Name), strings.TrimSpace(job.Category.Name), "", -1)

	intChapterNo, err := strconv.Atoi(strings.TrimSpace(chapterNo))

	if err != nil {
		log.Println(err)
		return
	}

	dbChapter := &DbChapter{
		Name:         ReplaceSpecial(job.Chapter.Name),
		Link:         job.Chapter.Link.String(),
		DbCategory:   *dbCategory,
		ChapterNo:    intChapterNo,
		TotalPages:   len(dbPages),
		Pages:        dbPages,
		ScrappedTime: time.Now().Unix(),
	}

	db.Create(dbChapter)

	log.Println("success when saving for ", dbChapter.Name)
}

func processPages(chapter Chapter) (dbPages []DbPage, err error) {
	pages, err := pagesFromChapter(chapter)

	if err != nil {
		log.Println(err)
		return
	}

	if err != nil {
		return dbPages, err
	}

	pageWorkerResults := make(chan PageWorkerResult, len(pages))

	for _, page := range pages {

		pageWorker(page, pageWorkerResults)
	}

	for i := 0; i < len(pages); i++ {
		r := <-pageWorkerResults
		if r.Err != nil {
			err = r.Err
			log.Println(err)
			return dbPages, err
		}

		dbPages = append(dbPages, r.Val)
	}

	return
}

func pageWorker(p Page, result chan<- PageWorkerResult) {

	mangaSrc, err := mangaSrcFromPage(p.Link)

	if err != nil || mangaSrc == nil {
		result <- PageWorkerResult{Val: DbPage{}, Err: err}
		return
	}

	imgb, err := watermark(mangaSrc)

	if err != nil {
		result <- PageWorkerResult{Val: DbPage{}, Err: err}
		return
	}

	uuid, err := uuid.NewV4()
	if err != nil {
		result <- PageWorkerResult{Val: DbPage{}, Err: err}
		return
	}

	hashCode := hash(uuid.String())
	bucketNum := hashCode % 100
	hostedMangaSrc := fmt.Sprintf("%v/%v.jpg", bucketNum, strings.Replace(uuid.String(), "-", "", -1))
	path := fmt.Sprintf("images/%v", hostedMangaSrc)

	file, err := os.Create(path)

	if err != nil {
		result <- PageWorkerResult{Val: DbPage{}, Err: err}
		return
	}

	_, err = file.Write(imgb)
	file.Close()

	log.Printf("written %v to disk \n", path)

	if err != nil {
		result <- PageWorkerResult{Val: DbPage{}, Err: err}
		return
	}

	absUrl := fmt.Sprintf("%v/%v", imageServer, path)
	mp := DbPage{MangaSrc: mangaSrc.String(), PageNo: p.PageNo, HostedMangaSrc: absUrl}

	result <- PageWorkerResult{Val: mp, Err: nil}
}

func newDocument(c http.Client, url string) (doc *goquery.Document, err error) {

	req, err := http.NewRequest("GET", url, nil)

	req.Close = true

	res, err := c.Do(req)
	if err != nil {
		log.Println(err)
		return
	}

	defer res.Body.Close()

	doc, err = goquery.NewDocumentFromReader(res.Body)

	if err != nil {
		log.Println(err)
		return
	}

	return
}

func mangaSrcFromPage(pageLink *url.URL) (url *url.URL, err error) {
	c := acquire()
	defer release(c)

	doc, err := newDocument(c, pageLink.String())
	if err != nil {
		return
	}

	docHTML, err := doc.Html()
	if err != nil {
		return
	}

	if docHTML == "<html><head></head><body><h1>404 Not Found</h1></body></html>" {
		err = errors.New("404 respons error from page")
		return
	}

	element := doc.Find("div#imgholder img#img").First()
	value, isExist := element.Attr("src")
	if !isExist {
		return nil, errors.New("cannot find img src on page")
	}

	url, err = url.Parse(value)
	if err != nil {
		return
	}

	return url, nil
}

func pagesFromChapter(chapter Chapter) (pages []Page, err error) {
	c := acquire()
	defer release(c)

	doc, err := newDocument(c, chapter.Link.String())
	if err != nil {
		log.Println(err)
		return pages, err
	}

	docHtml, err := doc.Html()

	if err != nil {
		log.Println(err)
		return pages, err
	}

	if docHtml == "<html><head></head><body><h1>404 Not Found</h1></body></html>" {
		err = errors.New("404 error when get pages of a chapter from mangareader")
		return pages, err
	}

	doc.Find("select option").Each(func(i int, element *goquery.Selection) {
		value, isExist := element.Attr("value")
		if isExist {
			link, err := url.Parse(root + value)
			if err == nil {
				page := Page{PageNo: i + 1, Link: link}
				pages = append(pages, page)
			}
		}
	})

	log.Println("found ", len(pages), " for ", chapter.Name)

	return
}

func getNewJobs(category Category) (out []ChapterJobContext) {
	fromSite, err := chaptersFromSite(category)

	if err != nil {
		log.Println(err)
		return
	}

	existingChapters, err := existingChaptersInDb(category)
	if err != nil {
		log.Println(err)
		return
	}

	newChapters := except(chapterNamesFromSite(fromSite), existingChapters)

	for _, chapter := range fromSite {
		if contains(newChapters, chapter.Name) {
			out = append(out, ChapterJobContext{Category: category, Chapter: chapter})
		}
	}

	return
}

func chapterNamesFromSite(chapters []Chapter) []string {

	out := make([]string, len(chapters))
	for i, chapter := range chapters {
		out[i] = strings.TrimSpace(chapter.Name)
	}
	return out
}

func getDbCategory(in Category) (out *DbCategory, err error) {
	c := acquire()
	defer release(c)

	dbCategory := &DbCategory{}

	queryCategoryName := ReplaceSpecial(in.Name)

	db.Where(&DbCategory{Name: queryCategoryName}).First(dbCategory)

	log.Printf("queried cat : %+v\n", dbCategory)

	if dbCategory.Name == queryCategoryName {
		out = dbCategory
		log.Println("found dbCategory in database ", out.Name)
		return
	}

	log.Println("dbCategory not found in database", in.Name)

	doc, err := newDocument(c, in.Link.String())
	if err != nil {
		return
	}

	categoryImgElement := doc.Find("div#mangaimg img").First()
	categoryImg, ok := categoryImgElement.Attr("src")
	if !ok {
		err := errors.New("cannot find category img")
		return nil, err
	}
	categoryImgUrl, err := url.Parse(categoryImg)
	if err != nil {
		return nil, err
	}

	altNameElement := doc.Find("div#mangaproperties table tbody tr:nth-child(2) td:nth-child(2)").First()
	altName := altNameElement.Text()

	yearOfReleaseElement := doc.Find("div#mangaproperties table tbody tr:nth-child(3) td:nth-child(2)").First()
	yearOfRelease := yearOfReleaseElement.Text()

	statusElement := doc.Find("div#mangaproperties table tbody tr:nth-child(4) td:nth-child(2)").First()
	status := statusElement.Text()

	authorElement := doc.Find("div#mangaproperties table tbody tr:nth-child(5) td:nth-child(2)").First()
	author := authorElement.Text()

	artistElement := doc.Find("div#mangaproperties table tbody tr:nth-child(6) td:nth-child(2)").First()
	artist := artistElement.Text()

	description := doc.Find("div#readmangasum p").First().Text()

	genres := make([]DbGenre, 0)
	doc.Find("div#mangaproperties table tbody tr:nth-child(8) td:nth-child(2) span").Each(func(i int, element *goquery.Selection) {
		g := DbGenre{Name: element.Text()}
		genres = append(genres, g)

	})

	hostedCategoryImage, err := hostCategoryImage(c, categoryImgUrl)

	if err != nil {
		return nil, err
	}

	toSave := &DbCategory{}

	toSave.HostedCategoryImage = hostedCategoryImage
	toSave.CategoryImage = categoryImgUrl.String()
	toSave.AltName = altName
	toSave.YearOfRelease = yearOfRelease
	toSave.Status = status
	toSave.Author = author
	toSave.Artist = artist
	toSave.Genres = genres
	toSave.Description = description
	toSave.Name = ReplaceSpecial(in.Name)
	toSave.Link = in.Link.String()

	db.Create(toSave)

	log.Println("saved category " + toSave.Name)

	out = toSave

	return
}

func hostCategoryImage(httpClient http.Client, source *url.URL) (out string, err error) {

	uuid, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	hashCode := hash(uuid.String())
	bucketNum := hashCode % 100
	hostedSrc := fmt.Sprintf("%v/%v.jpg", bucketNum, strings.Replace(uuid.String(), "-", "", -1))
	path := fmt.Sprintf("images/%v", hostedSrc)

	file, err := os.Create(path)

	if err != nil {
		return "", err
	}

	image, err := downloadImageWithClient(httpClient, source)

	if err != nil {
		return "", err
	}

	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, image, nil)

	if err != nil {
		return "", err
	}

	imgb := buf.Bytes()

	_, err = file.Write(imgb)
	file.Close()

	if err != nil {
		return "", err
	}

	absHostedSrc := fmt.Sprintf("%v/%v", imageServer, path)

	return absHostedSrc, nil
}

func chaptersFromSite(category Category) (chapters []Chapter, err error) {
	c := acquire()
	defer release(c)

	doc, err := newDocument(c, category.Link.String())
	if err != nil {
		log.Println(err)
		return chapters, err
	}

	doc.Find("table#listing a").Each(func(i int, element *goquery.Selection) {
		href, isExist := element.Attr("href")
		if isExist {
			link, err := url.Parse(root + href)
			if err == nil {
				chapter := Chapter{Name: ReplaceSpecial(element.Text()), Link: link}
				chapters = append(chapters, chapter)
			}
		}
	})

	return
}

func getCategoriesFromSite() (categories []Category, err error) {
	c := acquire()
	defer release(c)

	listing := root + "/alphabetical"

	doc, err := newDocument(c, listing)

	if err != nil {
		log.Println(err)
		return categories, err
	}
	doc.Find("ul.series_alpha li a").Each(func(i int, element *goquery.Selection) {
		href, isExist := element.Attr("href")
		if isExist {
			link, err := url.Parse(root + href)
			if err == nil {
				cat := Category{Name: ReplaceSpecial(element.Text()), Link: link}

				if err == nil {
					categories = append(categories, cat)
				}
			}
		}
	})

	log.Println(len(categories), " categories found from target site")

	return categories, nil
}

func existingChaptersInDb(category Category) (out []string, err error) {

	distinctChapters := make([]DbChapter, 0)
	db.Select("DISTINCT(Name)").Find(&distinctChapters)

	for _, chapter := range distinctChapters {
		out = append(out, chapter.Name)
	}

	return
}

func imageType(src *url.URL) (imageType string, err error) {
	if strings.HasSuffix(src.String(), ".png") {
		imageType = "png"
	} else if strings.HasSuffix(src.String(), ".jpg") {
		imageType = "jpg"
	} else {
		errorMsg := "only png and jpg is supported, received" + src.String()
		err = errors.New(errorMsg)
	}

	return
}

func downloadImage(src *url.URL) (image.Image, error) {
	client := acquire()
	defer release(client)
	return downloadImageWithClient(client, src)
}

func downloadImageWithClient(client http.Client, src *url.URL) (image.Image, error) {

	imageType, err := imageType(src)

	if err != nil {
		return nil, err
	}

	resp, err := client.Get(src.String())

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var img image.Image

	if imageType == "jpg" {
		img, err = jpeg.Decode(resp.Body)
	} else if imageType == "png" {
		img, err = png.Decode(resp.Body)
	}

	if err != nil {
		return nil, err
	}

	return img, nil
}

func watermark(src *url.URL) (out []byte, err error) {

	img, err := downloadImage(src)

	if err != nil {
		return
	}

	wmb, err := os.Open("watermark.png")
	defer wmb.Close()

	if err != nil {
		return
	}

	watermark, err := png.Decode(wmb)

	if err != nil {
		return
	}

	offset := image.Pt(10, 5)
	b := img.Bounds()
	m := image.NewRGBA(b)
	draw.Draw(m, b, img, image.ZP, draw.Src)
	draw.Draw(m, watermark.Bounds().Add(offset), watermark, image.ZP, draw.Over)

	w := new(bytes.Buffer)
	err = jpeg.Encode(w, m, &jpeg.Options{jpeg.DefaultQuality})

	if err != nil {
		return
	}

	out = w.Bytes()

	return
}

func hash(s string) uint32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	return h.Sum32()

}

func except(a []string, b []string) []string {
	out := make([]string, 0)
	for _, j := range a {

		if !contains(b, j) {
			out = append(out, j)
		}

	}

	return out

}

func feedsContainCategory(slice []CategoryFromFeedServer, category Category) bool {
	for _, i := range slice {
		if strings.TrimSpace(i.CategoryName) == strings.TrimSpace(category.Name) {
			return true
		}
	}
	return false
}

func contains(slice []string, s string) bool {
	for _, i := range slice {
		if strings.TrimSpace(i) == strings.TrimSpace(s) {
			return true
		}

	}
	return false
}

func shuffle(a []Category) {
	for i := range a {
		j := rand.Intn(i + 1)
		a[i], a[j] = a[j], a[i]
	}
}

func reverse(numbers []Category) []Category {
	for i, j := 0, len(numbers)-1; i < j; i, j = i+1, j-1 {
		numbers[i], numbers[j] = numbers[j], numbers[i]
	}
	return numbers
}
