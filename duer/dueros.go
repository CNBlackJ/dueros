package duer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"path"
	"strings"
	"time"

	"dueros/utils"

	// "github.com/tidwall/gjson"
	"github.com/icexin/dueros/auth"
	"github.com/icexin/dueros/proto"
	"github.com/twinj/uuid"
	"github.com/yanyiwu/gojieba"
)

const (
	DuerOSHost = "dueros-h2.baidu.com"
)

var (
	OS *DuerOS
)

func mustToken() string {
	token, err := auth.GetToken()
	if err != nil {
		panic(err)
	}
	return token
}

func requestURI(s string) string {
	p := path.Join("dcs/v1", s)
	return fmt.Sprintf("https://%s/%s", DuerOSHost, p)
}

type Registry interface {
	Dispatch(m *proto.Message) error
	Context() []*proto.Message
}

type DuerOS struct {
	c        *http.Client
	deviceid string

	eventch  chan *proto.Message
	directch chan *proto.Message

	registry Registry
	isSpeak  chan int
}

func NewDuerOS(r Registry) *DuerOS {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 1,
		},
	}

	d := &DuerOS{
		c:        client,
		deviceid: "icexin-dueros-" + uuid.NewV4().String(),
		eventch:  make(chan *proto.Message, 2),
		directch: make(chan *proto.Message, 2),
		registry: r,
		isSpeak:  make(chan int),
	}
	go d.handleDownChannelLoop()
	go d.handlePingLoop()
	go d.handleEventLoop()
	go d.handleDirectLoop()
	return d
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
			if b == a {
					return true
			}
	}
	return false
}

func (d *DuerOS) handlePingLoop() {
	ticker := time.NewTicker(time.Minute * 5)
	defer ticker.Stop()
	for range ticker.C {
		d.ping()
	}
}

func (d *DuerOS) handleDownChannelLoop() {
	for {
		resp, err := d.get("/directives")
		if err != nil {
			resp.Close()
			log.Printf("downchannel error:%s", err)
			time.Sleep(time.Second * 3)
			continue
		}
		d.handleResponse(resp)
	}
}

func (d *DuerOS) PostEvent(m *proto.Message) {
	d.eventch <- m
}

func (d *DuerOS) handleEventLoop() {
	for event := range d.eventch {
		resp, err := d.postEvent(event)
		if err == proto.ErrEmptyBody {
			continue
		}
		if err != nil {
			log.Print(err)
			continue
		}
		d.handleResponse(resp)
	}
}

func (d *DuerOS) handleDirectLoop() {
	for direct := range d.directch {
		err := d.registry.Dispatch(direct)
		if err != nil {
			log.Print(err)
		}
	}
}

func (d *DuerOS) ping() {
	resp, err := d.get("/ping")
	if err != nil {
		log.Print(err)
		return
	}
	resp.Close()
}

func (d *DuerOS) stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func (d *DuerOS) handleResponse(resp *proto.ResponseReader) {
	defer resp.Close()
	for {
		direct, err := resp.ReadDirective()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("%#v", err)
			continue
		}
		utils.SetKeyword("天气aa")
		if direct.PayloadJSON.Get("type").String() == "FINAL" {
			inputText := direct.PayloadJSON.Get("text")
	
			var s string
			var words []string
			use_hmm := true
			x := gojieba.NewJieba()
			defer x.Free()
		
			s = inputText.String()
			words = x.Cut(s, use_hmm)
			fmt.Println(s)
			fmt.Println("全模式匹配:", strings.Join(words, "/"))
			if stringInSlice("天气", words) {
				fmt.Println("===+启动天气服务+===")
				d.isSpeak <- 0
			} else {
				d.isSpeak <- 1
			}
			if stringInSlice("美食", words) {
				fmt.Println("===+启动美食服务+===")
				d.isSpeak <- 0
			}
			if stringInSlice("景点", words) {
				fmt.Println("===+启动景点服务+===")
				d.isSpeak <- 0
			}
			if stringInSlice("休闲", words) {
				fmt.Println("===+启动休闲服务+===")
				d.isSpeak <- 0
			}
		}
		kw := utils.GetKeyword()
		fmt.Println("====keyword====")
		fmt.Println(kw)
		log.Printf("directive: %s.%s:%s ", direct.Header.Namespace, direct.Header.Name, direct.PayloadJSON)
		if direct.Header.Namespace == "ai.dueros.device_interface.voice_output" &&
			direct.Header.Name == "Speak" {
			rc, err := resp.ReadAttach()
			if err != nil {
				log.Printf("read attach error:%s", err)
				continue
			}
			buf := new(bytes.Buffer)
			isSpeak := <- d.isSpeak
			if isSpeak == 1 {
				io.Copy(buf, rc)
			}
			rc.Close()
			direct.Attach = ioutil.NopCloser(buf)
		}
		d.directch <- direct
	}
}

func (d *DuerOS) get(method string) (*proto.ResponseReader, error) {
	req, err := http.NewRequest("GET", requestURI(method), nil)
	if err != nil {
		return nil, err
	}
	return d.doRequest(req)
}

func newMimeHeader(contentType, fieldName string) textproto.MIMEHeader {
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="%s"`, fieldName))
	h.Set("Content-Type", contentType)
	return h
}

func (d *DuerOS) postEvent(e *proto.Message) (*proto.ResponseReader, error) {
	msg := map[string]interface{}{
		"clientContext": d.registry.Context(),
		"event":         e,
	}
	buf, _ := json.Marshal(msg)
	log.Printf("==request:%s", buf)

	pr, pw := io.Pipe()
	w := multipart.NewWriter(pw)
	go func() {
		// write json metadata
		partWriter, _ := w.CreatePart(newMimeHeader("application/json", "metadata"))
		partWriter.Write(buf)

		// write audio attachment
		if e.Attach != nil {
			partWriter, _ = w.CreatePart(newMimeHeader("application/octet-stream", "audio"))
			_, err := io.CopyBuffer(partWriter, e.Attach, make([]byte, 320))
			if err != nil && err != io.EOF {
				log.Fatalf("%+v", err)
			}
		}
		// flush multipart content
		w.Close()

		// tell http client EOF of http body
		pw.Close()
	}()
	req, _ := http.NewRequest("POST", requestURI("/events"), pr)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return d.doRequest(req)
}

func (d *DuerOS) doRequest(req *http.Request) (*proto.ResponseReader, error) {
	req.Header.Set("dueros-device-id", d.deviceid)
	req.Header.Set("authorization", "Bearer "+mustToken())
	resp, err := d.c.Do(req)
	if err != nil {
		return nil, err
	}
	return proto.NewResponseReader(resp)
}
