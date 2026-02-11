package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type SizeGroup struct {
	Size  int
	Count int64
}

func main() {
	csvFile := flag.String("csv", "data_full.csv", "CSV file with size,count records")
	slotsStr := flag.String("slots", "", "Comma-separated slot sizes (source values, without +24)")
	pageSizeLog := flag.Int("pagelog", 20, "Page size log2 (16=64KB, 20=1MB)")
	headerSize := flag.Int("header", 40, "Page header size in bytes")
	sliceHeader := flag.Int("slice", 24, "Slice header added to each record size")

	flag.Parse()

	if *slotsStr == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -slots \"72,80,96,...\" [-csv file.csv] [-pagelog 20]\n", os.Args[0])
		os.Exit(1)
	}

	pageSize := 1 << *pageSizeLog
	pageAvail := pageSize - *headerSize

	// Parse slot sizes
	parts := strings.Split(*slotsStr, ",")
	var sourceSlots []int
	for _, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Invalid slot size: %q\n", p)
			os.Exit(1)
		}
		sourceSlots = append(sourceSlots, v)
	}
	sort.Ints(sourceSlots)

	// Apply +sliceHeader (like init() does)
	slots := make([]int, len(sourceSlots))
	for i, s := range sourceSlots {
		slots[i] = s + *sliceHeader
	}
	maxShared := slots[len(slots)-1]

	// Load CSV
	groups := loadCSV(*csvFile, *sliceHeader)

	// Route records to classes (mimicking getSizeClass: size >= maxShared -> dedicated)
	classCounts := make([]int64, len(slots))
	var dedicatedCount int64
	var totalRecords int64

	for _, g := range groups {
		totalRecords += g.Count
		if g.Size >= maxShared {
			dedicatedCount += g.Count
			continue
		}
		for ci, s := range slots {
			if g.Size <= s {
				classCounts[ci] += g.Count
				break
			}
		}
	}

	// Print results
	fmt.Printf("Page size: %d (%dKB), Page avail: %d, Header: %d, Slice header: %d\n",
		pageSize, pageSize/1024, pageAvail, *headerSize, *sliceHeader)
	fmt.Printf("Classes: %d, MaxSharedSize: %d (source: %d)\n",
		len(slots), maxShared, sourceSlots[len(sourceSlots)-1])
	fmt.Printf("Total records in CSV: %d\n\n", totalRecords)

	fmt.Printf("%-5s %7s %7s %6s %12s %6s %12s %12s\n",
		"Class", "Source", "Slot", "SPP", "Count", "Pages", "SlotWaste", "LastPgFree")
	fmt.Println(strings.Repeat("-", 80))

	var totalPages int64
	var totalSlotWaste int64
	var totalLastPgFree int64
	gi := 0

	for ci, s := range slots {
		spp := pageAvail / s
		var pages int64
		if classCounts[ci] > 0 {
			pages = (classCounts[ci] + int64(spp) - 1) / int64(spp)
		}

		// Calculate slot waste for this class
		var classSlotWaste int64
		var upperBound int
		if ci < len(slots)-1 {
			upperBound = slots[ci+1] - 1
		} else {
			upperBound = maxShared - 1
		}
		for gi < len(groups) && groups[gi].Size <= upperBound && groups[gi].Size < maxShared {
			g := groups[gi]
			if g.Size <= s {
				classSlotWaste += int64(s-g.Size) * g.Count
			}
			gi++
		}

		// Last page free slots
		var lastPgFree int64
		if classCounts[ci] > 0 {
			usedInLast := classCounts[ci] % int64(spp)
			if usedInLast == 0 {
				usedInLast = int64(spp)
			}
			lastPgFree = (int64(spp) - usedInLast) * int64(s)
		}

		totalPages += pages
		totalSlotWaste += classSlotWaste
		totalLastPgFree += lastPgFree

		fmt.Printf("%5d %7d %7d %6d %12d %6d %12d %12d\n",
			ci, sourceSlots[ci], s, spp, classCounts[ci], pages, classSlotWaste, lastPgFree)
	}

	fmt.Println(strings.Repeat("-", 80))
	fmt.Printf("Shared pages:    %d\n", totalPages)
	fmt.Printf("Dedicated pages: %d (records with size >= %d)\n", dedicatedCount, maxShared)
	totalPages += dedicatedCount
	fmt.Printf("TOTAL PAGES:     %d (%d MB)\n", totalPages, totalPages*int64(pageSize)>>20)
	fmt.Printf("Slot waste:      %d bytes (%.1f MB)\n", totalSlotWaste, float64(totalSlotWaste)/(1024*1024))
	fmt.Printf("Last-page free:  %d bytes (%.1f MB)\n", totalLastPgFree, float64(totalLastPgFree)/(1024*1024))
}

func loadCSV(filename string, sliceHeader int) []SizeGroup {
	f, err := os.Open(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening CSV: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	allRecords, err := reader.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}

	var records []SizeGroup
	for i, row := range allRecords {
		if i == 0 {
			continue
		}
		size, _ := strconv.Atoi(strings.TrimSpace(row[0]))
		count, _ := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
		size += sliceHeader
		size = (size + 7) &^ 7
		records = append(records, SizeGroup{Size: size, Count: count})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Size < records[j].Size
	})

	// Merge duplicates
	merged := make([]SizeGroup, 0, len(records))
	for _, r := range records {
		if len(merged) > 0 && merged[len(merged)-1].Size == r.Size {
			merged[len(merged)-1].Count += r.Count
		} else {
			merged = append(merged, r)
		}
	}
	return merged
}
