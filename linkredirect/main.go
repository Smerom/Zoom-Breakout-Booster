package main

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/autotls"
	"github.com/gin-gonic/gin"
)

type templData struct {
	CountPer int
	URLs     []string
}

var templ string = `
<html>
<body> 

<p>Redirects per URL: {{.CountPer}}</p>
<p>Current URLs:</p>
{{range .URLs}}
<p>{{.}}</p>
{{end}}
<br>
<form method="post">
<label for="count">Redirect Count to each URL:</label>
<br>
<input type="number" id="count" name="countPer" required>
<br>
<label for="urls">URLs (separated by new line):</label>
<br>
<textarea id="urls" name="urlList" rows="10" cols="100" required>
</textarea>
<br>
<button>Set Redirect</button>
</form>

</body>
</html>
`

var index = template.Must(template.New("Main").Parse(templ))

func Index(w http.ResponseWriter, r *http.Request) {
	count, urls := masterAllocator.Info()
	data := templData{
		CountPer: count,
		URLs:     urls,
	}
	err := index.Execute(w, data)
	if err != nil {
		fmt.Printf("Error: %s", err)
	}
}

func Set(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	defer r.Body.Close()
	defer Index(w, r)
	if err != nil {
		fmt.Printf("Error: %s", err)
		return
	}
	var countPer int
	if strCount, ok := r.PostForm["countPer"]; ok {
		if len(strCount) == 0 {
			fmt.Printf("Count not found")
			return
		}
		intCount, err := strconv.ParseInt(strCount[0], 10, 32)
		if err != nil {
			fmt.Printf("Error: %s", err)
			return
		}
		countPer = int(intCount)
	} else {
		fmt.Printf("Count not found")
		return
	}

	var urlList string
	if urlListList, ok := r.PostForm["urlList"]; ok {
		if len(urlListList) == 0 {
			fmt.Printf("url list not found")
			return
		}
		urlList = urlListList[0]
	} else {
		fmt.Printf("url list not found")
		return
	}

	// split URLList on new line
	urlStrings := strings.Split(urlList, "\n")
	var urls []string
	for _, urlString := range urlStrings {
		trimmed := strings.TrimSpace(urlString)
		if _, err = url.ParseRequestURI(trimmed); err == nil {
			urls = append(urls, trimmed)
		} else {
			fmt.Printf("Error: %s", err)
		}
	}

	allocator := &LinkAllocator{
		CountPer: countPer,
		URLs:     urls,
	}

	masterAllocator = allocator
}

var masterAllocator *LinkAllocator

func Redirect(w http.ResponseWriter, r *http.Request) {
	url, err := masterAllocator.Next()
	if err != nil {
		fmt.Printf("Error: %s", err)
		w.WriteHeader(http.StatusNotFound)
		return
	}

	http.Redirect(w, r, url, http.StatusSeeOther)
}

func main() {
	router := gin.Default()

	router.Any("/", gin.WrapF(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("https://%s%s", r.Host, r.RequestURI), http.StatusMovedPermanently)
	}))
	router.GET("/redirectLink", gin.WrapF(Redirect))

	go func() {
		log.Fatal(http.ListenAndServe(":80", router))
	}()

	admin := gin.Default()
	// Ping handler
	admin.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	authorized := admin.Group("/", gin.BasicAuth(gin.Accounts{
		"admin": "NQ8xcYVeQvBEjtma6OOw",
	}))
	authorized.GET("/", gin.WrapF(Index))
	authorized.POST("/", gin.WrapF(Set))

	log.Fatal(autotls.Run(admin, "redirect.quinnmueller.me"))
}

type LinkAllocator struct {
	sync.Mutex
	CountPer     int
	CurrentCount int
	URLs         []string
}

func (l *LinkAllocator) Info() (count int, urls []string) {
	if l == nil {
		return
	}
	count = l.CountPer
	urls = l.URLs
	return
}

func (l *LinkAllocator) Next() (url string, err error) {
	if l == nil {
		err = errors.New("bad allocator")
		return
	}
	l.Lock()
	defer l.Unlock()

	bin := l.CurrentCount / l.CountPer

	if bin < len(l.URLs) {
		url = l.URLs[bin]
	} else {
		err = errors.New("no more urls left")
	}

	fmt.Printf("\nCurrent Count: %d, \nBin: %d\nURL: %s\n", l.CurrentCount, bin, url)

	l.CurrentCount++

	return
}
