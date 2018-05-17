package audio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"time"
	"os"

	"dueros/utils"

	"github.com/skratchdot/open-golang/open"
	// "github.com/zserge/webview"
	"github.com/bobertlo/go-mpg123/mpg123"
	"github.com/chekun/baidu-yuyin/asr"
	"github.com/chekun/baidu-yuyin/oauth"
)

type Player struct {
	Writer *Writer
}

func NewPlayer() *Player {
	return &Player{}
}

func (p *Player) TransferToWav(fn string, folder string, timeUnix int64) (outFn string, err error) {
	outFn = fmt.Sprintf("%s/mofun%d.wav", folder, timeUnix)
	cmd := exec.Command("ffmpeg", "-i", fn, outFn)
	_, err = cmd.Output()
	if err != nil { fmt.Println(err) }
	return
}

func (p *Player) TranslateToText(fn string) (text string, err error) {
	clientID := "S7L7gTPjImsd6uHnVw3ryG6i"
	clientSecret := "lcMLr7BlhXuKEcjWzfUoKkiqvQLLmRHi"
	auth := oauth.New(clientID, clientSecret, oauth.NewMemoryCacheMan())
	token, err := auth.GetToken()
	if err != nil { fmt.Println(err) }
	file, err := os.Open(fn)
	if err != nil { fmt.Println(err) }
	defer file.Close()
	text, err = asr.ToText(token, file)
	if err != nil { fmt.Println(err) }
	return
}

func (p *Player) ShowText(text string) (err error) {
	host := "http://192.168.1.67"
	port := "8080"
	path := "weather?weather="
	url := fmt.Sprintf("%s:%s/%s%s", host, port, path, text)
	fmt.Println(url)
	open.Start(url)
	return
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func (p *Player) LoadMP3Reader(r io.Reader) (*Writer, error) {
	// save file
	timeUnix := time.Now().Unix()
	folder := "./storage"
	fn := fmt.Sprintf("%s/mofun%d.mp3", folder, timeUnix)
	buf := new(bytes.Buffer)
	io.Copy(buf, r)
	buf.ReadFrom(r)
	ioutil.WriteFile(fn, buf.Bytes(), 0644)

	kw := utils.GetKeyword()
	fmt.Println("==keyword==")
	fmt.Println(kw)

	// transfer to wav
	outFn, _ := p.TransferToWav(fn, folder, timeUnix)

	// 根据不同的定制服务，返回不同的录音；如：美食/出行

	// translate to text
	text, _ := p.TranslateToText(outFn)

	// call api to show text
	p.ShowText(text)

	return p.loadMP3File(fn)
}

func (p *Player) LoadMP3(uri string) (*Writer, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http", "https":
		resp, err := http.Get(uri)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return p.LoadMP3Reader(resp.Body)
	case "", "file":
		return p.loadMP3File(u.Path)
	}
	return nil, errors.New("bad uri: " + uri)
}

func (p *Player) LoadAndPlay(uri string) error {
	w, err := p.LoadMP3(uri)
	if err != nil {
		return err
	}
	defer w.Close()
	return w.Play()
}

func (p *Player) loadMP3File(file string) (*Writer, error) {
	d, err := mpg123.NewDecoder("")
	if err != nil {
		return nil, err
	}
	defer d.Close()
	err = d.Open(file)
	if err != nil {
		return nil, err
	}
	rate, channels, encoding := d.GetFormat()
	log.Printf("rate:%d, channel:%d, encoding:%d", rate, channels, encoding)

	buf := new(bytes.Buffer)
	io.Copy(buf, d)

	return NewWriter(int(rate), channels, buf.Bytes())
}
