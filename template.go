package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var TemplateCache *template.Template

func RenderTemplate(w http.ResponseWriter, tmpl string, title string, member Member, local interface{}) {
	fmt.Println("Render '" + tmpl + "' for " + member.Name)

	master := TemplateCache.Lookup("master")
	global := struct {
		Config Configuration
		Member Member
	}{
		Config: GlobalConfig,
		Member: member,
	}

	if master == nil {
		http.Error(w, "Master template not found", 404)
	} else {
		page := TemplateCache.Lookup(tmpl)

		if page == nil {
			http.Error(w, "Template '"+tmpl+"' not found or invalid", 500)
		} else {
			buf := new(bytes.Buffer)

			err := page.Execute(buf, local)
			if err != nil {
				http.Error(w, "Page template execution failed: "+err.Error(), 500)
			} else {
				data := struct {
					Title  string
					Body   template.HTML
					Global interface{}
					Local  interface{}
				}{
					Title:  title,
					Body:   template.HTML(buf.String()),
					Global: global,
					Local:  local,
				}

				err := master.Execute(w, data)

				if err != nil {
					http.Error(w, "Master template execution failed: "+err.Error(), 500)
				}
			}
		}
	}
}

func InitializeTemplates() error {
	funcs := template.FuncMap{
		"isGuest":     IsGuest,
		"formatDate":  FormatDate,
		"formatMoney": FormatMoney,
		"prefix":      GlobalPrefix,
	}

	TemplateCache = template.New("all").Funcs(funcs)

	log.Println("Load templates from '" + GlobalConfig.Templates + "'")

	var walkFn func(string, os.FileInfo, error) error
	walkFn = func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
			return err
		} else {
			if !info.IsDir() && filepath.Base(path)[0:1] != "." {
				log.Println("Load '" + path + "'")

				_, err = TemplateCache.ParseFiles(path)
				if err != nil {
					log.Println(err)
				}
				return err
			}
		}

		return nil
	}

	err := filepath.Walk(GlobalConfig.Templates, walkFn)
	if err != nil {
		return err
	}

	if TemplateCache.Lookup("index") == nil {
		return errors.New("No 'index' template found")
	}

	return nil
}

func IsGuest(member Member) bool {
	return member.Id == 0
}

func FormatDate(unix int64) string {
	return time.Unix(unix, 0).Format(time.UnixDate)
}

func FormatMoney(cents uint64) string {
	return fmt.Sprintf("%0.2f", float64(cents)/100.0)
}

func GlobalPrefix() string {
	return GlobalConfig.Location
}
