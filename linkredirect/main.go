package main

import (
	"errors"
	"flag"
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

var user, password, host string

func init() {
	flag.StringVar(&user, "u", "", "username for basic auth")
	flag.StringVar(&password, "p", "", "password for basic auth")
	flag.StringVar(&host, "host", "", "host name")
	flag.Parse()

	// check values
	if user == "" {
		log.Fatal("Missing username for basic auth (-u)")
	}
	if password == "" {
		log.Fatal("Missing password for basic auth (-p)")
	}

	// assume host is valid if present
	if host == "" {
		log.Fatal("Missing host (-host)")
	}
}

type templData struct {
	CountPer  int
	URLs      []UrlInfo
	Host      string
	GroupSize int
}

type UrlInfo struct {
	URL      string
	Count    int
	MaxCount int
	Active   bool
}

var templ string = `
<html>
<script>
window.addEventListener( "load", function () {
	function sendData() {
	  const XHR = new XMLHttpRequest();
  
	  // Bind the FormData object and the form element
	  const FD = new FormData( form );
  
	  // Define what happens on successful data submission
	  XHR.addEventListener( "load", function(event) {
		window.location.reload();
	  } );
  
	  // Define what happens in case of error
	  XHR.addEventListener( "error", function( event ) {
		alert( 'Oops! Something went wrong.' );
	  } );
  
	  // Set up our request
	  XHR.open( form.method, form.action );
  
	  // The data sent is what the user provided in the form
	  XHR.send( FD );
	}
   
	// Access the form element...
	const form = document.getElementById( "redirectForm" );
  
	// ...and take over its submit event.
	form.addEventListener( "submit", function ( event ) {
	  event.preventDefault();
  
	  sendData();
	} );
  } );
</script>
<body> 

<h2>Current Setup:</h2>
<p>Primary redirect URL:  http://{{.Host}}/redirectLink</p>
<p>Group Size: {{.GroupSize}}</p>
<table>
<tr><th>URL</th><th>Count</th></tr>
{{- range .URLs}}
<tr{{if .Active}} style="font-weight:bold"{{end}}><td>{{.URL}}</td><td>({{.Count}}/{{.MaxCount}})</td></tr>
{{- end}}
</table>
<br>
<h2>Create New:</h2>
<p>Enter the number of users to be sent to each subsequent URL. And a list of URLs separated by newlines.</p>
<p>Each URL will be "filled" in the order entered. Resubmitting the form with the same info will reset the user counter to the beginning.</p>
<form method="POST" id="redirectForm">
<label for="count">Redirect Count to each URL:</label>
<br>
<input type="number" id="count" name="countPer" value="{{.CountPer}}" required>
<br>
<label for="count">Group size (should be divisor of the Redirect Count):</label>
<br>
<input type="number" id="groupSize" name="groupSize" value="{{.GroupSize}}" required>
<br>
<label for="urls">URLs (separated by new line):</label>
<br>
<textarea id="urls" name="urlList" rows="10" cols="100" required>
{{- range .URLs}}
{{.URL}}
{{- end -}}
</textarea>
<br>
<button>Set Redirect</button>
</form>

</body>
</html>
`

var index = template.Must(template.New("Main").Parse(templ))

func Index(w http.ResponseWriter, r *http.Request) {
	current, count, groupSize, urls := masterAllocator.Info()

	var urlData []UrlInfo
	if count > 0 && len(urls) > 0 {
		activeGroup := current / groupSize
		activeBin := activeGroup % len(urls)
		for index, url := range urls {
			// find cound from full groups
			var urlCount int
			urlCount = activeGroup / len(urls) * groupSize
			if activeGroup%len(urls)-index > 0 {
				urlCount += (activeGroup%len(urls) - index) * groupSize
			}

			var active bool
			if index == activeBin {
				active = true
				// add current group
				urlCount += current % groupSize
			}

			urlData = append(urlData, UrlInfo{
				URL:      url,
				Count:    urlCount,
				MaxCount: count,
				Active:   active,
			})
		}
	}

	data := templData{
		CountPer:  count,
		GroupSize: groupSize,
		URLs:      urlData,
		Host:      host,
	}
	err := index.Execute(w, data)
	if err != nil {
		log.Printf("Error: %s", err)
	}
}

func Set(w http.ResponseWriter, r *http.Request) {
	err := r.ParseMultipartForm(1000000)
	defer r.Body.Close()
	defer Index(w, r)
	if err != nil {
		log.Printf("Error: %s", err)
		return
	}
	var countPer int
	log.Printf("Post Form: %#v", r.PostForm)
	log.Printf("Form: %#v", r.Form)
	if strCount, ok := r.PostForm["countPer"]; ok {
		if len(strCount) == 0 {
			log.Printf("Count not found")
			return
		}
		intCount, err := strconv.ParseInt(strCount[0], 10, 32)
		if err != nil {
			log.Printf("Error: %s", err)
			return
		}
		countPer = int(intCount)
	} else {
		log.Printf("Count not found")
		return
	}

	var groupSize int
	if strGroupSize, ok := r.PostForm["groupSize"]; ok {
		if len(strGroupSize) == 0 {
			log.Printf("GroupSize not found")
			return
		}
		intCount, err := strconv.ParseInt(strGroupSize[0], 10, 32)
		if err != nil {
			log.Printf("Error: %s", err)
			return
		}
		groupSize = int(intCount)
	} else {
		log.Printf("GroupSize not found")
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
		CountPer:  countPer,
		GroupSize: groupSize,
		URLs:      urls,
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
		http.Redirect(w, r, fmt.Sprintf("https://%s/", host), http.StatusMovedPermanently)
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
		user: password,
	}))
	authorized.GET("/", gin.WrapF(Index))
	authorized.POST("/", gin.WrapF(Set))

	log.Fatal(autotls.Run(admin, host))
}

type LinkAllocator struct {
	sync.Mutex
	CountPer     int
	GroupSize    int
	CurrentCount int
	URLs         []string
}

func (l *LinkAllocator) Info() (currentCount, countPer, groupSize int, urls []string) {
	if l == nil {
		return
	}
	l.Lock()
	defer l.Unlock()
	currentCount = l.CurrentCount
	countPer = l.CountPer
	urls = l.URLs
	groupSize = l.GroupSize
	return
}

func (l *LinkAllocator) Next() (url string, err error) {
	if l == nil {
		err = errors.New("bad allocator")
		return
	}
	// could be done faster with an atomic add
	l.Lock()
	defer l.Unlock()

	// check past end
	if l.CurrentCount >= len(l.URLs)*l.CountPer {
		err = errors.New("no more urls left")
		return
	}

	// find group
	group := l.CurrentCount / l.GroupSize

	// find bin
	bin := group % len(l.URLs)

	if bin < len(l.URLs) {
		url = l.URLs[bin]
	} else {
		err = errors.New("no more urls left")
	}

	fmt.Printf("\nCurrent Count: %d, \nBin: %d\nURL: %s\n", l.CurrentCount, bin, url)

	l.CurrentCount++

	return
}
