package database

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"drazil.de/go64u/config"
	"drazil.de/go64u/network"
	"drazil.de/go64u/util"
	"github.com/spf13/cobra"
)

type Category struct {
	Id           int    `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Description  string `json:"description"`
	GroupingName string `json:"groupingName"`
}

type Preset struct {
	Type        string  `json:"type"`
	Description string  `json:"description"`
	Values      []Value `json:"values"`
}

type Value struct {
	AqlKey string `json:"aqlKey"`
	Name   string `json:"name"`
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

var categories []Category
var presets []Preset
var compoTypes []CompoType
var entries []Entry

var selectedCategory = -1

func Cache() {
	result := network.Get(fmt.Sprintf("%s/categories", config.GetConfig().ResourceUrl))

	json.Unmarshal(result, &categories)
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].Id < categories[j].Id
	})

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
	replCmd.AddCommand(quitCommand())
	replCmd.AddCommand(categoriesCommand())
	replCmd.AddCommand(listCommand())
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

func listCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list filtered results",
		Run: func(cmd *cobra.Command, args []string) {

			offset := 0
			limit := 30
			var query strings.Builder
			if selectedCategory > -1 {
				query.WriteString(fmt.Sprintf("(%s:\"%s\")", "category", categories[selectedCategory].Name))
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

			for i, category := range categories {
				selected := util.Reset
				if i == selectedCategory {
					selected = util.Green
				}
				fmt.Printf("%s[%02d] - %s%s\n", selected, category.Id, category.Name, util.Reset)
			}
			scanner := bufio.NewScanner(os.Stdin)
			for {
				prompt()
				if !scanner.Scan() {
					break
				}
				command := scanner.Text()
				if command == "q" {
					break
				}
				if command == "r" {
					selectedCategory = -1
					break
				}
				if util.IsNumber(command) {
					id, _ := strconv.Atoi(command)
					var err error
					selectedCategory, err = getCategory(id)
					if err != nil {
						fmt.Println(err)
					} else {
						fmt.Printf("selected category: %s\n", categories[selectedCategory].Name)
						break
					}
				} else {
					fmt.Println("not a number")
				}
			}
		},
	}
}

func getCategory(id int) (int, error) {
	ci := -1
	found := false
	for i, c := range categories {
		if c.Id == id {
			ci = i
			found = true
			break
		}
	}
	if !found {
		return -1, fmt.Errorf("unkown category id: %d", id)
	}

	return ci, nil
}
