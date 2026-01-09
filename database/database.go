package database

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"drazil.de/go64u/config"
	"drazil.de/go64u/network"
	"drazil.de/go64u/util"
	"github.com/spf13/cobra"
)

/*
	type Category struct {
		Id           int    `json:"id"`
		Name         string `json:"name"`
		Type         string `json:"type"`
		Description  string `json:"description"`
		GroupingName string `json:"groupingName"`
	}
*/
type Preset struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Values      []Value `json:"values"`
}

type Value struct {
	AqlKey   string `json:"aqlKey"`
	Name     string `json:"name"`
	Selected bool   `json:"-"`
}

type CompoType struct {
	Id       int    `json:"id"`
	Name     string `json:"name"`
	Selected bool   `json:"selected"`
}

type ContentContainer struct {
	ContentEntry []File `json:"contentEntry"`
}

type File struct {
	Id   int    `json:"id"`
	Size int    `json:"size"`
	Path string `json:"path"`
}

type Package struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	Category     int    `json:"category"`
	SiteCategory int    `json:"siteCategory"`
	SiteRanking  int    `json:"siteRanking"`
	Group        string `json:"group"`
	Year         int    `json:"year"`
	Rating       int    `json:"rating"`
	Updated      string `json:"updated"`
	Selected     bool   `json:"-"`
}

// var categories []Category
var presets []Preset
var compoTypes []CompoType
var entries []Package

var reFlag = regexp.MustCompile(`--(\w+)`)
var reParam = regexp.MustCompile(`(\w+)`)

type Filter struct {
	Name        string
	Group       string
	Handle      string
	Category    string
	SubCategory string
	Repository  string
	Year        string
	Rating      string
	Type        string
	Latest      string
}

var contentContainer ContentContainer
var filter Filter
var offset int
var limit int
var ignoreDefaults bool
var get bool

func DownloadCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "query",
		Short:   "query packages matching to a filter",
		Long:    "query packages matching to a filter\nBy default is filter by current year and type=d64",
		GroupID: "platform",
		Args:    cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			var queryFilter string
			var err error
			var doQuery = true
			queryFilter, err = buildFilter(filter)
			if err != nil {
				doQuery = false
			}
			if doQuery {
				queryResult(queryFilter)
				for _, pac := range entries {
					id, _ := strconv.Atoi(pac.Id)
					readEntries(id, pac.Category)
					for _, entry := range contentContainer.ContentEntry {
						saveEntry(id, pac.Category, entry.Path, filter.Type)
					}
					//fmt.Printf("%07s:%05d:%-40s\n", entry.Id, entry.Category, entry.Name)
				}
			}
		},
	}
	cmd.Flags().StringVarP(&filter.Name, "name", "", "", "")
	cmd.Flags().StringVarP(&filter.Group, "group", "", "", "")
	cmd.Flags().StringVarP(&filter.Handle, "handle", "", "", "")
	cmd.Flags().StringVarP(&filter.Category, "category", "", "", "")
	cmd.Flags().StringVarP(&filter.Repository, "repo", "", "", "")
	cmd.Flags().StringVarP(&filter.SubCategory, "subcat", "", "", "")
	cmd.Flags().StringVarP(&filter.Year, "year", "", "", "")
	cmd.Flags().StringVarP(&filter.Rating, "rating", "", "", "")
	cmd.Flags().StringVarP(&filter.Type, "type", "", "", "")
	cmd.Flags().StringVarP(&filter.Latest, "latest", "", "", "")
	cmd.Flags().IntVarP(&offset, "offset", "", 0, "")
	cmd.Flags().IntVarP(&limit, "limit", "", 40, "")
	cmd.Flags().BoolVarP(&ignoreDefaults, "ignoreDefaults", "", false, "")
	cmd.Flags().BoolVarP(&get, "get", "", false, "get the files")

	return cmd
}

func Cache() {
	//result := network.HttpGet(fmt.Sprintf("%s/categories", config.GetConfig().ResourceUrl))
	/*
		json.Unmarshal(result, &categories)
		sort.Slice(categories, func(i, j int) bool {
			return categories[i].Id < categories[j].Id
		})
	*/
	/*
		result := network.SendHttpRequest(&network.HttpConfig{
			URL:         fmt.Sprintf("%s/aql/presets", config.GetConfig().ResourceUrl),
			Method:      http.MethodGet,
			SetClientId: true,
		})
		json.Unmarshal(result, &presets)

		result = network.SendHttpRequest(&network.HttpConfig{
			URL:         fmt.Sprintf("%s/compotypes", config.GetConfig().ResourceUrl),
			Method:      http.MethodGet,
			SetClientId: true,
		})
		json.Unmarshal(result, &compoTypes)
		sort.Slice(compoTypes, func(i, j int) bool {
			return compoTypes[i].Id < compoTypes[j].Id
		})
	*/
}

func readEntries(id int, categoryId int) {
	result := network.SendHttpRequest(&network.HttpConfig{
		URL:         fmt.Sprintf("%s/entries/%d/%d", config.GetConfig().ResourceUrl, id, categoryId),
		Method:      http.MethodGet,
		SetClientId: true,
	})
	json.Unmarshal(result, &contentContainer)
}

func saveEntry(id int, categoryId int, fileName string, fileType string) {
	content := network.SendHttpRequest(&network.HttpConfig{
		URL:         fmt.Sprintf("%s/bin/%d/%d/%s", config.GetConfig().ResourceUrl, id, categoryId, fileName),
		Method:      http.MethodGet,
		SetClientId: true,
	})

	if strings.HasSuffix(fileName, fileType) || fileType == "" {
		if get {
			s := fmt.Sprintf("%s%s", config.GetConfig().DownloadFolder, fileName)
			fmt.Println(s)
			f, err := os.Create(s)
			if err != nil {
				fmt.Println("error creating file")
			}
			defer f.Close()

			bytesWritten, err := f.Write(content)
			if err != nil {
				fmt.Println("error writing file")
			}
			fmt.Printf("wrote %d bytes\n", bytesWritten)
		} else {
			fmt.Println(fileName)
		}
	}
}

func Run() {
	fmt.Println("Welcome to the go64u database(a64) mode! Type 'quit' to exit.")
	replCmd := &cobra.Command{}
	replCmd.CompletionOptions.DisableDefaultCmd = true
	replCmd.AddCommand(categoriesCommand())
	replCmd.AddCommand(filterCommand())
	replCmd.AddCommand(listCommand())
	replCmd.AddCommand(quitCommand())
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Printf("%s: ", util.White)
		if !scanner.Scan() {
			break
		}
		commandLine := scanner.Text()
		args := strings.Fields(commandLine)
		replCmd.SetArgs(args)

		if err := replCmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		util.ResetAllFlags(replCmd)
	}
}

func quitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "quit",
		Short: "quit terminal",
		Run: func(cmd *cobra.Command, args []string) {
			log.Println("leave terminal mode")
			os.Exit(0)
		},
	}
}

func prompt() {
	fmt.Print("select category id (q=quit or r=reset selection): ")
}

func filterCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "filter",
		Short: "filter",
		Run: func(cmd *cobra.Command, args []string) {
			scanner := bufio.NewReader(os.Stdin)
			for {
				/*
					if !scanner.Scan() {
						break
					}
				*/
				command, _, _ := scanner.ReadLine()
				group := findLastFlag(string(command))
				fmt.Println(group)
				autocomplete(string(command), findGroupValues(group))
			}
		},
	}
	return cmd
}
func findLastFlag(command string) string {

	parts := reFlag.FindAllStringSubmatch(command, 30)
	if parts == nil {
		return ""
	}
	lastGroup := parts[len(parts)-1]
	return lastGroup[len(lastGroup)-1]
}
func findGroupValues(groupName string) []Value {
	for _, p := range presets {
		if p.Type == groupName {
			return p.Values
		}
	}
	return nil
}

func autocomplete(command string, values []Value) {
	parts := reParam.FindAllString(command, 30)
	if parts == nil {
		return
	}
	lastValue := parts[len(parts)-1]
	for _, v := range values {
		if strings.HasPrefix(v.Name, lastValue) {
			fmt.Println("autocomplete match")
		}
	}

}
func buildFilter(filter Filter) (string, error) {
	var query strings.Builder
	if filter.Name != "" {
		query.WriteString(buildFilterItem("name", filter.Name, true))
	}
	if filter.Group != "" {
		query.WriteString(buildFilterItem("group", filter.Group, true))
	}
	if filter.Handle != "" {
		query.WriteString(buildFilterItem("handle", filter.Handle, true))
	}
	if filter.Category != "" {
		query.WriteString(buildFilterItem("category", filter.Category, false))
	}
	if filter.SubCategory != "" {
		query.WriteString(buildFilterItem("subcat", filter.SubCategory, false))
	}
	if filter.Year != "" {
		query.WriteString(buildFilterItem("date", filter.Year, false))
	} else {
		if !ignoreDefaults {
			query.WriteString(buildFilterItem("date", strconv.Itoa(time.Now().Year()), false))
		}

	}
	if filter.Rating != "" {
		query.WriteString(buildFilterItem("rating", filter.Rating, true))
	}
	if filter.Type != "" {
		query.WriteString(buildFilterItem("type", filter.Type, false))
	}
	if filter.Latest != "" {
		query.WriteString(buildFilterItem("latest", filter.Latest, false))
	} else {
		if !ignoreDefaults {
			query.WriteString(buildFilterItem("latest", "1month", false))
		}
	}
	var err error
	var queryString = query.String()

	if queryString == "" {
		err = fmt.Errorf("at least one filter value is needed\n")
	} else {
		queryString = queryString[0 : len(queryString)-3]
	}
	return queryString, err
}

func buildFilterItem(name string, value string, quote bool) string {
	if quote {
		return fmt.Sprintf("(%s:\"%s\") & ", name, value)
	} else {
		return fmt.Sprintf("(%s:%s) & ", name, value)
	}
}

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list filtered results",
		Run: func(cmd *cobra.Command, args []string) {

			offset := 0
			limit := 30
			var query string
			var err error
			var doList = true
			value := getSelectedPreset("category")
			if nil != value {
				query, err = buildFilter(Filter{Category: value.AqlKey, Year: "2025"})
				if err != nil {
					doList = false
				}
			}
			scanner := bufio.NewScanner(os.Stdin)
			stopScanning := false
			doQuery := true
			for !stopScanning {

				if doQuery && doList {
					queryResult(query)
				}
				for i := range 30 {
					if i < len(entries) {
						fmt.Printf("\033[2K\033[97m[%02d] | %-30s | %-20s | %04d|%s\n", i, crop(entries[i].Name, 30), crop(entries[i].Group, 20), entries[i].Year, printRating(entries[i].Rating))
					} else {
						fmt.Printf("\033[2K\033[97m[%02d] - empty slot\n", i)
					}
				}
				fmt.Printf("\n\033[2K[P]revious page [N]ext page [S]elect(n) [F]ilter [Q]uit: ")
				if !scanner.Scan() {
					break
				}
				command := scanner.Text()
				switch command {
				case "p":
					{
						if offset > 0 {
							offset -= limit
							doQuery = true
						}
						pageUp()
					}
				case "n":
					{
						if len(entries) > 0 && len(entries) == 30 {
							offset += limit
							doQuery = true
						}
						pageUp()
					}
				case "q":
					{
						doQuery = false
						stopScanning = true
						fmt.Println()
						break
					}
				default:
					{
						doQuery = false
						pageUp()
					}
				}

			}
		}}
}
func queryResult(query string) {
	result := network.SendHttpRequest(&network.HttpConfig{
		URL:         fmt.Sprintf("%s/aql/%d/%d?query=%s", config.GetConfig().ResourceUrl, offset, limit, url.QueryEscape(query)),
		Method:      http.MethodGet,
		SetClientId: true,
	})
	json.Unmarshal(result, &entries)
}

func printRating(rating int) string {
	var sb strings.Builder
	for i := range 10 {
		if i <= rating && rating > 0 {
			sb.WriteString("\033[33m★\033[0m")

		} else {
			sb.WriteString("\033[90m☆\033[0m")
		}
	}
	return sb.String()
}

func pageUp() {
	fmt.Print("\033[K\033[32A\r")
}
func crop(s string, length int) string {
	if len([]rune(s)) < length-3 {
		return s
	}
	return s[0:length-3] + "..."
}

func categoriesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "category",
		Short: "show categories",
		Run: func(cmd *cobra.Command, args []string) {

			for {
				for i, category := range getPresetsByKey("category") {
					highlight := util.Reset
					if category.Selected {
						highlight = util.Yellow
					}
					fmt.Printf("%s[%02d] - %s%s\n", highlight, i, category.Name, util.Reset)
				}
				scanner := bufio.NewScanner(os.Stdin)

				prompt()
				if !scanner.Scan() {
					break
				}
				command := scanner.Text()
				if command == "q" {
					break
				}
				if command == "r" {
					getSelectedPreset("category").Selected = false
					break
				}
				if util.IsNumber(command) {
					id, _ := strconv.Atoi(command)
					var err error
					var name string
					name, err = setSelectedPreset("category", id)
					if err != nil {
						fmt.Println(err)
					} else {
						fmt.Printf("selected category: %s\n", name)
						//break
					}
				} else {
					fmt.Println("not a number")
				}
				fmt.Printf("\033[%02dA\r", len(getPresetsByKey("category"))+2)
			}
		},
	}
}

func getPresetsByKey(key string) []Value {
	for i := range presets {
		if presets[i].Type == key {
			return presets[i].Values
		}
	}
	return nil
}

func getSelectedPreset(key string) *Value {
	values := getPresetsByKey(key)
	for i := range values {
		if values[i].Selected {
			return &values[i]
		}
	}
	return nil
}

func setSelectedPreset(key string, id int) (string, error) {
	for i := range presets {
		if presets[i].Type == key {
			if id >= len(presets[i].Values) {
				return "", fmt.Errorf("unknown %s id: %d", key, id)
			}

			for j := range presets[i].Values {
				presets[i].Values[j].Selected = (j == id)
			}

			return presets[i].Values[id].Name, nil
		}
	}
	return "", fmt.Errorf("unknown preset type: %s", key)
}
