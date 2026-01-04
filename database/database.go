package database

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

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

type Entry struct {
	Id           string `json:"id"`
	Name         string `json:"name"`
	Category     int    `json:"category"`
	SiteCategory int    `json:"siteCcategory"`
	SiteRanking  int    `json:"siteRanking"`
	Group        string `json:"group"`
	Year         int    `json:"year"`
	Rating       int    `json:"rating"`
	Updated      string `json:"updated"`
}

// var categories []Category
var presets []Preset
var compoTypes []CompoType
var entries []Entry

var reFlag = regexp.MustCompile(`--(\w+)`)
var reParam = regexp.MustCompile(`(\w+)`)

func Cache() {
	result := network.Get(fmt.Sprintf("%s/categories", config.GetConfig().ResourceUrl))
	/*
		json.Unmarshal(result, &categories)
		sort.Slice(categories, func(i, j int) bool {
			return categories[i].Id < categories[j].Id
		})
	*/
	result = network.Get(fmt.Sprintf("%s/aql/presets", config.GetConfig().ResourceUrl))
	json.Unmarshal(result, &presets)
	result = network.Get(fmt.Sprintf("%s/compotypes", config.GetConfig().ResourceUrl))
	json.Unmarshal(result, &compoTypes)
	sort.Slice(compoTypes, func(i, j int) bool {
		return compoTypes[i].Id < compoTypes[j].Id
	})
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

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list filtered results",
		Run: func(cmd *cobra.Command, args []string) {

			offset := 0
			limit := 30
			var query strings.Builder
			value := getSelectedPreset("category")
			if nil != value {
				query.WriteString(fmt.Sprintf("(%s:\"%s\")", "category", value.Name))
			}
			scanner := bufio.NewScanner(os.Stdin)
			for {
				s := fmt.Sprintf("%s/aql/%d/%d?query=%s", config.GetConfig().ResourceUrl, offset, limit, url.PathEscape(query.String()))
				result := network.Get(s)
				json.Unmarshal(result, &entries)
				for i, entry := range entries {
					fmt.Printf("[%02d] - %-30s - %-20s - Year:%04d - Rating:%02d\n", i, crop(entry.Name, 30), crop(entry.Group, 20), entry.Year, entry.Rating)
				}
				fmt.Printf("\n[P]revious page [N]ext page [ESC]: ")
				if !scanner.Scan() {
					break
				}
				command := scanner.Text()
				switch command {
				case "p":
					{
						if offset > 0 {
							offset -= limit
						}
					}
				case "n":
					{
						offset += limit
					}
				default:
					{
					}
				}
				fmt.Print("\033[K\033[32A\r")
			}
		}}
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
