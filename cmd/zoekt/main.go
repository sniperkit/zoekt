/*
Sniperkit-Bot
- Date: 2018-08-12 12:11:26.372554071 +0200 CEST m=+0.045728207
- Status: analyzed
*/

// Copyright 2016 Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	// external
	jsoniter "github.com/sniperkit/snk.golang.json/pkg/json/v1"
	// internal
	"github.com/sniperkit/snk.fork.zoekt/pkg"
	"github.com/sniperkit/snk.fork.zoekt/pkg/query"
	"github.com/sniperkit/snk.fork.zoekt/pkg/shards"
)

const CONTEXT = 20

var (
	json = jsoniter.ConfigCompatibleWithStandardLibrary
)

type JSONOutput struct {
	Options struct {
		Query   string `json:"query"`
		Context int    `json:"context"`
	} `json:"options"`
	Results []*JSONResult `json:"results"`
}

type JSONResult struct {
	FileName    string  `json:"filename"`
	Score       float64 `json:"score"`
	LineContent string  `json:"line_content"`
	LineNumber  int     `json:"line_number"`
}

func displayJSONMatches(files []zoekt.FileMatch, pat string) error {
	input := &JSONOutput{}
	input.Options.Context = CONTEXT
	input.Results = make([]*JSONResult, 0)
	for _, f := range files {
		for _, m := range f.LineMatches {
			result := &JSONResult{f.FileName, m.Score, string(m.Line), m.LineNumber}
			input.Results = append(input.Results, result)
		}
	}
	output, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		return err
	}
	os.Stdout.Write(output)
	os.Exit(1)
	return nil
}

func displayMatches(files []zoekt.FileMatch, pat string) {
	for _, f := range files {
		for _, m := range f.LineMatches {
			fmt.Printf("%s:%d:%s\n", f.FileName, m.LineNumber, m.Line)
		}
	}
}

func loadShard(fn string) (zoekt.Searcher, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}

	iFile, err := zoekt.NewIndexFile(f)
	if err != nil {
		return nil, err
	}
	s, err := zoekt.NewSearcher(iFile)
	if err != nil {
		iFile.Close()
		return nil, fmt.Errorf("NewSearcher(%s): %v", fn, err)
	}

	return s, nil
}

func main() {
	displayFormat := flag.String("displayFormat", "json", "output results")

	shard := flag.String("shard", "", "search in a specific shard")
	index := flag.String("index_dir",
		filepath.Join(os.Getenv("HOME"), ".zoekt"), "search for index files in `directory`")
	cpuProfile := flag.String("cpu_profile", "", "write cpu profile to `file`")
	profileTime := flag.Duration("profile_time", time.Second, "run this long to gather stats.")
	verbose := flag.Bool("v", false, "print some background data")

	flag.Usage = func() {
		name := os.Args[0]
		fmt.Fprintf(os.Stderr, "Usage:\n\n  %s [option] QUERY\n"+
			"for example\n\n  %s 'byte file:java -file:test'\n\n", name, name)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
	}
	flag.Parse()

	if len(flag.Args()) == 0 {
		fmt.Fprintf(os.Stderr, "Pattern is missing.\n")
		flag.Usage()
		os.Exit(2)
	}
	pat := flag.Arg(0)

	var searcher zoekt.Searcher
	var err error
	if *shard != "" {
		searcher, err = loadShard(*shard)
	} else {
		searcher, err = shards.NewDirectorySearcher(*index)
	}

	if err != nil {
		log.Fatal(err)
	}

	query, err := query.Parse(pat)
	if err != nil {
		log.Fatal(err)
	}
	if *verbose {
		log.Println("query:", query)
	}

	var sOpts zoekt.SearchOptions
	sres, err := searcher.Search(context.Background(), query, &sOpts)
	if *cpuProfile != "" {
		// If profiling, do it another time so we measure with
		// warm caches.
		f, err := os.Create(*cpuProfile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		if *verbose {
			log.Println("Displaying matches...")
		}

		t := time.Now()
		pprof.StartCPUProfile(f)
		for {
			sres, err = searcher.Search(context.Background(), query, &sOpts)
			if time.Now().Sub(t) > *profileTime {
				break
			}
		}
		pprof.StopCPUProfile()
	}

	if err != nil {
		log.Fatal(err)
	}

	switch *displayFormat {
	case "json":
		if err := displayJSONMatches(sres.Files, pat); err != nil {
			log.Fatal(err)
		}
	case "text":
		fallthrough
	default:
		displayMatches(sres.Files, pat)
	}

	if *verbose {
		log.Printf("stats: %#v", sres.Stats)
	}
}
