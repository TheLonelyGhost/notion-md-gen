package generator

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bonaysoft/notion-md-gen/pkg/tomarkdown"

	"github.com/dstotijn/go-notion"
)

func Run(config Config) error {
	if err := os.MkdirAll(config.Markdown.PostSavePath, 0755); err != nil {
		return fmt.Errorf("couldn't create content folder: %s", err)
	}

	// find database page
	client := notion.NewClient(os.Getenv("NOTION_SECRET"))
	q, err := queryDatabase(client, config.Notion)
	if err != nil {
		return fmt.Errorf("❌ Querying Notion database: %s", err)
	}
	fmt.Println("✔ Querying Notion database: Completed")

	// fetch page children
	changed := 0 // number of article status changed
	for i, page := range q.Results {
		pageName := tomarkdown.ConvertRichText(page.Properties.(notion.DatabasePageProperties)["Name"].Title)
		fmt.Printf("-- Article [%d/%d] %s --\n", i+1, len(q.Results), pageName)

		// Get page blocks tree
		blocks, err := queryBlockChildren(client, page.ID)
		if err != nil {
			log.Println("❌ Getting blocks tree:", err)
			continue
		}
		fmt.Println("✔ Getting blocks tree: Completed")

		// Generate content to file
		if err := generate(page, blocks, config.Markdown); err != nil {
			fmt.Println("❌ Generating blog post:", err)
			continue
		}
		fmt.Println("✔ Generating blog post: Completed")

		// Change status of blog post if desired
		if changeStatus(client, page, config.Notion) {
			changed++
		}
	}

	// Set GITHUB_ACTIONS info variables
	// https://docs.github.com/en/actions/learn-github-actions/workflow-commands-for-github-actions
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		fmt.Printf("::set-output name=articles_published::%d\n", changed)
	}

	return nil
}

func generate(page notion.Page, blocks []notion.Block, config Markdown) error {
	// Create file
	pageName := config.PageNamePrefix + tomarkdown.ConvertRichText(page.Properties.(notion.DatabasePageProperties)["Name"].Title)
	f, err := os.Create(filepath.Join(config.PostSavePath, generateArticleFilename(pageName, page.CreatedTime, config)))
	if err != nil {
		return fmt.Errorf("error create file: %s", err)
	}

	// Generate markdown content to the file
	tm := tomarkdown.New()
	tm.ImgSavePath = filepath.Join(config.ImageSavePath, pageName)
	tm.ImgVisitPath = filepath.Join(config.ImagePublicLink, url.PathEscape(pageName))
	tm.ContentTemplate = config.Template
	tm.WithFrontMatter(page)
	if config.ShortcodeSyntax != "" {
		tm.EnableExtendedSyntax(config.ShortcodeSyntax)
	}

	return tm.GenerateTo(blocks, f)
}

// generateArticleFilename creates a filesystem-safe slug from the title of the article
func generateArticleFilename(title string, date time.Time, config Markdown) (escapedFilename string) {
	// Replace characters not explicitly allowed with a placeholder (hyphen)
	slugReplacementChar := "-"
	disallowedSlugChars := regexp.MustCompile("[^A-Za-z0-9_.-]+")
	slugReplacementTrimmer := regexp.MustCompile(fmt.Sprintf("^%s+|%s+-$", slugReplacementChar, slugReplacementChar))
	slugReplacementDeduper := regexp.MustCompile(fmt.Sprintf("%s+", slugReplacementChar))

	escapedTitle := strings.ToLower(title)
	escapedTitle = strings.ToValidUTF8(escapedTitle, "")
	// Disallow most characters, but allow alphanumeric (plus -_.) in filename
	escapedTitle = disallowedSlugChars.ReplaceAllLiteralString(escapedTitle, slugReplacementChar)
	// Dedupe `---` in URL to just `-`, since it may have been `-@'` originally
	escapedTitle = slugReplacementDeduper.ReplaceAllLiteralString(escapedTitle, slugReplacementChar)
	// Leading and trailing `-` can lead to problems, and it just looks ugly. Remove it.
	escapedTitle = slugReplacementTrimmer.ReplaceAllLiteralString(escapedTitle, "")

	escapedFilename = escapedTitle + ".md"

	if config.GroupByMonth {
		escapedFilename = filepath.Join(date.Format("2006-01-02"), escapedFilename)
	}

	return
}
