package models

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type customResponse struct {
	StatusCode int

	Body []byte

	Header http.Header
}

type (
	Queue struct {
		top    *node
		rear   *node
		length int
	}

	node struct {
		pre   *node
		next  *node
		value any
	}
)

func SetCache(url string, response *http.Response, content []byte, cachePath string) error {
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "charset") {
		contentType := response.Header.Get("Content-Type")
		contentPartArr := strings.Split(contentType, ";")
		response.Header.Set("Content-Type", contentPartArr[0]+"; charset=utf-8")
	}
	response.Header.Del("Content-Encoding")
	dir, _ := filepath.Split(cachePath + url)

	if !isExist(dir) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	filename := path.Join(cachePath, url)
	//if find := strings.Contains(filename, ".php"); find {
	//	filename = strings.Replace(filename, ".php", ".html", 1)
	//}
	file, err := os.Create(filename + ".cache")
	if err != nil {

		return err
	}
	defer file.Close()
	_, err = io.WriteString(file, string(content))
	if err != nil {
		return err
	}
	return nil
}

func SetImgCache(url string, content []byte, cachePath string) error {
	dir, _ := filepath.Split(cachePath + url)
	if !isExist(dir) {
		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	filename := path.Join(cachePath, url)
	file, err := os.Create(filename)
	if err != nil {

		return err
	}
	defer file.Close()
	_, err = io.Copy(file, bytes.NewReader(content))
	return nil
}

func isExist(path string) bool {
	_, err := os.Stat(path)
	if err != nil {
		return os.IsExist(err)
	}
	return true

}

func NewQueue() *Queue {
	return &Queue{nil, nil, 0}
}

func (q *Queue) Len() int {
	return q.length
}

func (q *Queue) Any() bool {
	return q.length > 0
}

func (q *Queue) Peek() any {
	if q.top == nil {
		return nil
	}
	return q.top.value
}

func (q *Queue) Push(v any) {
	n := &node{nil, nil, v}
	if q.length == 0 {
		q.top = n
		q.rear = q.top
	} else {
		n.pre = q.rear
		q.rear.next = n
		q.rear = n
	}
	q.length++
}

func (q *Queue) Pop() any {
	if q.length == 0 {
		return nil
	}
	n := q.top
	if q.top.next == nil {
		q.top = nil
	} else {
		q.top = q.top.next
		q.top.pre.next = nil
		q.top.pre = nil
	}
	q.length--
	return n.value
}

func (q *Queue) Clear() {
	*q = Queue{}
}
