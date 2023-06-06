package main

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unsafe"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"go.abhg.dev/goldmark/wikilink"

	"github.com/goodsign/monday"

	_ "embed"
)

var _t1, _t2 *template.Template

//go:embed index.tmpl.html
var _idxTmpl string

//go:embed post.tmpl.html
var _postTmpl string

func init() {
	_t1 = template.New("post")
	_t1 = template.Must(_t1.Parse(_postTmpl))
	_t2 = template.New("index")
	_t2 = template.Must(_t2.Parse(_idxTmpl))
}

//go:embed public _redirects
var _fs embed.FS

var _md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
		extension.Typographer,
		&wikilink.Extender{},
	),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
)

var _name = "Bjorn Pagen"

func main() {
	if err := run(); err != nil {
		panic(err)
	}
}

func run() error {
	gitPath := os.Args[1]
	err := os.Chdir(gitPath)
	if err != nil {
		return err
	}
	outPath := "dist"

	var in []string
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(path, ".") {
			return nil
		}
		if strings.HasPrefix(path, outPath+"/") {
			return nil
		}
		if info.IsDir() {
			return nil
		}

		in = append(in, path)

		return nil
	})
	if err != nil {
		return fmt.Errorf("walk: %v", err)
	}

	err = os.MkdirAll(outPath, os.ModePerm)
	if err != nil {
		return err
	}

	err = copyDirFromEmbed(".", outPath)
	if err != nil {
		return err
	}

	ps := make(map[monday.Locale][]string)
	for _, inPath := range in {
		strs := strings.SplitN(inPath, "/", 2)
		title := strings.ReplaceAll(strs[0], "-", " ")
		localeStr, ok := strings.CutSuffix(strs[1], ".md")
		if !ok {
			return errors.New("not md file")
		}
		locale := monday.Locale(localeStr)

		if _, ok := ps[locale]; !ok {
			ps[locale] = []string{title}
			continue
		}

		ps[locale] = append(ps[locale], title)
	}

	for k, v := range ps {
		err = genIndex(k, v, outPath)
		if err != nil {
			return err
		}
	}

	for _, inPath := range in {
		err := genPost(inPath, outPath)
		if err != nil {
			return err
		}
	}

	return nil
}

type Index struct {
	Author string
	Locale monday.Locale
	Titles []string
}

func (i Index) Language() string {
	return string(i.Locale)[:2]
}

func genIndex(l monday.Locale, s []string, outRoot string) error {
	d := Index{_name, l, s}

	localeOut := filepath.Join(outRoot, string(l))
	err := os.MkdirAll(localeOut, os.ModePerm)
	if err != nil {
		return err
	}

	out, err := os.Create(filepath.Join(localeOut, "index.html"))
	if err != nil {
		return err
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_t2.Execute(out, d)

	err = out.Sync()
	if err != nil {
		return nil
	}

	return err
}

func genPost(path, outRoot string) error {
	strs := strings.SplitN(path, "/", 2)
	title := strings.ReplaceAll(strs[0], "-", " ")
	localeStr, ok := strings.CutSuffix(strs[1], ".md")
	if !ok {
		return errors.New("not md file")
	}
	locale := monday.Locale(localeStr)

	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	t, err := creationDate(path)
	if err != nil {
		return err
	}

	var sb strings.Builder
	if err := _md.Convert(source, &sb); err != nil {
		return err
	}

	post := Post{
		_name,
		t,
		title,
		sb.String(),
		locale,
	}

	localeOut := filepath.Join(outRoot, string(locale))
	err = os.MkdirAll(localeOut, os.ModePerm)
	if err != nil {
		return err
	}

	out, err := os.Create(filepath.Join(localeOut, title+".html"))
	if err != nil {
		return err
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_t1.Execute(out, post)

	err = out.Sync()
	if err != nil {
		return nil
	}

	return err
}

type Post struct {
	Author  string
	Created time.Time
	Title   string
	Content string
	Locale  monday.Locale
}

func (p Post) Language() string {
	return string(p.Locale)[:2]
}

func (p Post) CreatedHuman() string {
	return monday.Format(p.Created, "January 2 2006", p.Locale)
}

func (p Post) CreatedDateTime() string {
	return p.Created.Format(time.DateTime) + "Z"
}

func creationDate(path string) (t time.Time, err error) {
	var args = []string{
		"log",
		"--diff-filter=A",
		"--format=%ct",
		"--",
		path,
	}

	b, err := exec.Command("git", args...).Output()
	if err != nil {
		return t, fmt.Errorf("cmd output: %v", err)
	}
	s := strings.TrimSpace(yoloString(b))

	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return t, fmt.Errorf("find commit: %v", err)
	}

	t = time.Unix(i, 0)
	return t, nil
}

func copyFileFromEmbed(src, dst string) (err error) {
	in, err := _fs.Open(src)
	if err != nil {
		return
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	_, err = io.Copy(out, in)
	if err != nil {
		return
	}

	err = out.Sync()
	if err != nil {
		return
	}

	return
}

func copyDirFromEmbed(src string, dst string) (err error) {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	entries, err := _fs.ReadDir(src)
	if err != nil {
		return err
	}

	err = os.MkdirAll(dst, os.ModePerm)
	if err != nil {
		return
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			err = copyDirFromEmbed(srcPath, dstPath)
			if err != nil {
				return
			}
		} else {
			err = copyFileFromEmbed(srcPath, dstPath)
			if err != nil {
				return
			}
		}
	}

	return
}

func yoloString(b []byte) string {
	return *((*string)(unsafe.Pointer(&b)))
}
