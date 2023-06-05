package main

import (
	"embed"
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

//go:embed post.tmpl.html
var _postTmpl string

//go:embed public
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

	for _, f := range in {
		err := genFile(f, outPath)
		if err != nil {
			return nil
		}
	}

	return nil
}

func genFile(path, outRoot string) error {
	strs := strings.SplitN(path, "/", 2)
	origTitle := strs[0]
	title := strings.ReplaceAll(origTitle, "-", " ")
	locale := monday.Locale(strs[1])

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

	out, err := os.Create(filepath.Join(localeOut, origTitle+".html"))
	if err != nil {
		return err
	}
	defer func() {
		if e := out.Close(); e != nil {
			err = e
		}
	}()

	t1 := template.New("post")
	t1 = template.Must(t1.Parse(_postTmpl))

	t1.Execute(out, post)

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
		return t, fmt.Errorf("get cmd output: %v", err)
	}
	s := strings.TrimSpace(yoloString(b))

	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return t, fmt.Errorf("parse int: %v", err)
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
