package main

import (
	"bytes"
	"regexp"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	nurl "net/url"
	"os"
	"strconv"
	"strings"

	readability "github.com/go-shiori/go-readability"
	"github.com/spf13/cobra"

	"github.com/tdewolff/minify/v2"
    "github.com/tdewolff/minify/v2/html"

)

const index = `<!DOCTYPE HTML>
<html>
 <head>
  <meta charset="utf-8">
  <title>go-readability</title>
 </head>
 <body style="font-family:sans-serif; font-size:18px; color:#d3d3d3; background:#1e1e1e">
 <form action="/" style="width:80%">
  <fieldset>
   <legend>получить удобочитаемый контент</legend>
   <p><label for="url">URL </label><input type="url" name="url" placeholder="https://ссылка.на/страницу" style="width:90%;background:#252526;color:#d3d3d3;border:1px solid #d3d3d3;outline:none"></p>
   <p><input type="checkbox" name="text" value="true">только текст без форматирования</p>
   <p><input type="checkbox" name="metadata" value="true">получить только метаданные страницы</p>
  </fieldset>
  <p><input style="background:#252526;color:#d3d3d3;border:1px solid #d3d3d3;outline:none" type="submit" value="показать >>"></p>
 </form>
 </body>
</html>`

func main() {
	rootCmd := &cobra.Command{
		Use:   "go-readability [flags] [source]",
		Run:   rootCmdHandler,
		Short: "go-readability — это парсер для получения читабельного содержимого веб-страницы",
		Long: "go-readability — это парсер для получения читабельного содержимого веб-страницы.\n" +
			"источником может быть поток stdin, URL-адрес, html-текст или существующий файл на диске.",
	}

	rootCmd.Flags().StringP("http", "l", "", "запустить HTTP-сервер по указанному адресу и порту, например: -l 127.0.0.1:8888")
	rootCmd.Flags().BoolP("metadata", "m", false, "вывод только метаданных страницы")
	rootCmd.Flags().BoolP("text", "t", false, "вывод только неформатированного текста страницы")
	rootCmd.Flags().StringP("html", "c", "", "HTML-контент в виде строки")
	rootCmd.Flags().BoolP("stdin", "i", false, "HTML-контент в виде потока stdin")

	err := rootCmd.Execute()
	if err != nil {
		log.Fatalln(err)
	}
}

func rootCmdHandler(cmd *cobra.Command, args []string) {
    httpListen, _ := cmd.Flags().GetString("http")
    if httpListen != "" {
        http.HandleFunc("/", httpHandler)
        log.Println("запущен HTTP-сервер на", httpListen)
        log.Fatal(http.ListenAndServe(httpListen, nil))
        return
    }
    metadataOnly, _ := cmd.Flags().GetBool("metadata")
    textOnly, _ := cmd.Flags().GetBool("text")
    htmlContent, _ := cmd.Flags().GetString("html")
    useStdin, _ := cmd.Flags().GetBool("stdin")
    if useStdin {
        content, err := getContentFromStdin(metadataOnly, textOnly)
        if err != nil {
            log.Fatalln(err)
        }
        fmt.Println(content)
        return
    }

    if htmlContent != "" {
        content, err := getContentFromHTML(htmlContent, metadataOnly, textOnly)
        if err != nil {
            log.Fatalln(err)
        }
        fmt.Println(content)
        return
    }
    if len(args) > 0 {
        content, err := getContent(args[0], metadataOnly, textOnly)
        if err != nil {
            log.Fatalln(err)
        }
        fmt.Println(content)
        return
    }
    _ = cmd.Help()
}

func httpHandler(w http.ResponseWriter, r *http.Request) {
	metadataOnly, _ := strconv.ParseBool(r.URL.Query().Get("metadata"))
	textOnly, _ := strconv.ParseBool(r.URL.Query().Get("text"))
	url := r.URL.Query().Get("url")
	if url == "" {
		if _, err := w.Write([]byte(index)); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		log.Println("process URL", url)
		content, err := getContent(url, metadataOnly, textOnly)
		if err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if metadataOnly {
			w.Header().Set("Content-Type", "application/json")
		} else if textOnly {
			w.Header().Set("Content-Type", "text/plain")
		}
		if _, err := w.Write([]byte(content)); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}

func getContentFromHTML(htmlContent string, metadataOnly, textOnly bool) (string, error) {
    pageURL, _ := nurl.ParseRequestURI("http://fakehost.com")
    srcReader := strings.NewReader(htmlContent)
    buf := bytes.NewBuffer(nil)
    tee := io.TeeReader(srcReader, buf)
    if !readability.Check(tee) {
        return "", fmt.Errorf("не удалось спарсить страницу: страница нечитаемая")
    }

    article, err := readability.FromReader(buf, pageURL)
    if err != nil {
        return "", fmt.Errorf("не удалось спарсить страницу: %v", err)
    }

    if metadataOnly {
        metadata := map[string]interface{}{
            "title":   article.Title,
            "byline":  article.Byline,
            "excerpt": article.Excerpt,
            "image":   article.Image,
            "favicon": article.Favicon,
        }

        prettyJSON, err := json.MarshalIndent(&metadata, "", "    ")
        if err != nil {
            return "", fmt.Errorf("не удалось записать метаданные: %v", err)
        }

        return string(prettyJSON), nil
    }

    if textOnly {
        return article.TextContent, nil
    }

    minifiedHTML, err := minifyHTML(article.Content)
    if err != nil {
        return "", err
    }

    return minifiedHTML, nil
}


func getContent(srcPath string, metadataOnly, textOnly bool) (string, error) {
    var (
        pageURL   *nurl.URL
        srcReader io.Reader
    )

    if _, isURL := validateURL(srcPath); isURL {
        resp, err := http.Get(srcPath)
        if err != nil {
            return "", fmt.Errorf("не удалось получить страницу по ссылке: %v", err)
        }
        defer resp.Body.Close()

        pageURL = resp.Request.URL
        srcReader = resp.Body
    } else {
        srcFile, err := os.Open(srcPath)
        if err != nil {
            return "", fmt.Errorf("не удалось открыть файл по указанному пути: %v", err)
        }
        defer srcFile.Close()

        pageURL, _ = nurl.ParseRequestURI("http://fakehost.com")
        srcReader = srcFile
    }

    buf := bytes.NewBuffer(nil)
    tee := io.TeeReader(srcReader, buf)

    if !readability.Check(tee) {
        return "", fmt.Errorf("не удалось спарсить страницу: страница нечитаемая")
    }

    article, err := readability.FromReader(buf, pageURL)
    if err != nil {
        return "", fmt.Errorf("не удалось спарсить страницу: %v", err)
    }

    if metadataOnly {
        metadata := map[string]interface{}{
            "title":   article.Title,
            "byline":  article.Byline,
            "excerpt": article.Excerpt,
            "image":   article.Image,
            "favicon": article.Favicon,
        }

        prettyJSON, err := json.MarshalIndent(&metadata, "", "    ")
        if err != nil {
            return "", fmt.Errorf("не удалось записать метаданные: %v", err)
        }

        return string(prettyJSON), nil
    }

    if textOnly {
        return article.TextContent, nil
    }

    minifiedHTML, err := minifyHTML(article.Content)
    if err != nil {
        return "", err
    }

    return minifiedHTML, nil
}


func getContentFromStdin(metadataOnly, textOnly bool) (string, error) {
    buf := bytes.NewBuffer(nil)
    if _, err := io.Copy(buf, os.Stdin); err != nil {
        return "", fmt.Errorf("не удалось прочитать данные из потока stdin: %v", err)
    }
    pageURL, _ := nurl.ParseRequestURI("http://fakehost.com")
    srcReader := bytes.NewReader(buf.Bytes())
    if !readability.Check(srcReader) {
        return "", fmt.Errorf("не удалось спарсить страницу: страница нечитаемая")
    }

    article, err := readability.FromReader(bytes.NewReader(buf.Bytes()), pageURL)
    if err != nil {
        return "", fmt.Errorf("не удалось спарсить страницу: %v", err)
    }
    if metadataOnly {
        metadata := map[string]interface{}{
            "title":   article.Title,
            "byline":  article.Byline,
            "excerpt": article.Excerpt,
            "image":   article.Image,
            "favicon": article.Favicon,
        }

        prettyJSON, err := json.MarshalIndent(&metadata, "", "    ")
        if err != nil {
            return "", fmt.Errorf("не удалось записать метаданные: %v", err)
        }

        return string(prettyJSON), nil
    }

    if textOnly {
        return article.TextContent, nil
    }

    minifiedHTML, err := minifyHTML(article.Content)
    if err != nil {
        return "", err
    }

    return minifiedHTML, nil
}

func validateURL(path string) (*nurl.URL, bool) {
	url, err := nurl.ParseRequestURI(path)
	return url, err == nil && strings.HasPrefix(url.Scheme, "http")
}

func minifyHTML(input string) (string, error) {
    m := minify.New()
    m.AddFunc("text/html", html.Minify)
    normalizedInput := normalizeWhitespace(input)
    var buf bytes.Buffer
    err := m.Minify("text/html", &buf, strings.NewReader(normalizedInput))
    if err != nil {
        return "", fmt.Errorf("не удалось минифицировать HTML: %v", err)
    }

    return buf.String(), nil
}

func normalizeWhitespace(input string) string {
    re := regexp.MustCompile(`(?s)>([^<]+)<`)
    return re.ReplaceAllStringFunc(input, func(match string) string {
        text := strings.ReplaceAll(match, "\n", " ")
        text = strings.ReplaceAll(text, "\r", " ")
        return text
    })
}