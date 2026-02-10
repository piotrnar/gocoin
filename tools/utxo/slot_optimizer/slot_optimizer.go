package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SizeGroup struct {
	Size  int
	Count int64
}

func main() {
	csvFile := flag.String("csv", "data_full.csv", "CSV file with size,count records")
	numClasses := flag.Int("classes", 33, "Number of desired size classes")
	pageSizeLog := flag.Int("pagelog", 20, "Page size log2 (16=64KB, 20=1MB)")
	headerSize := flag.Int("header", 40, "Page header size in bytes")
	sliceHeader := flag.Int("slice", 24, "Slice header added to each record size")
	maxSlot := flag.Int("maxslot", 32748, "Maximum slot size (32724+24)")
	maxCandPerEndpoint := flag.Int("maxcand", 50, "Max candidates to try per endpoint")

	flag.Parse()

	pageSize := 1 << *pageSizeLog
	pageAvail := pageSize - *headerSize

	fmt.Printf("Page size: %d (%dKB), Page avail: %d, Header: %d, Slice header: %d\n",
		pageSize, pageSize/1024, pageAvail, *headerSize, *sliceHeader)
	fmt.Printf("Max slot: %d, Desired classes: %d\n", *maxSlot, *numClasses)

	groups := loadCSV(*csvFile, *sliceHeader, *maxSlot)
	fmt.Printf("Loaded %d distinct size groups\n", len(groups))

	var totalCount int64
	for _, g := range groups {
		totalCount += g.Count
	}
	fmt.Printf("Total records: %d\n", totalCount)

	candidates := buildCandidates(groups, pageAvail, *maxSlot)
	fmt.Printf("Total candidate pool: %d\n", len(candidates))

	t0 := time.Now()
	bestSlots, bestPages := dpOptimize(groups, candidates, *numClasses, pageAvail, *maxSlot, *maxCandPerEndpoint)
	elapsed := time.Since(t0)

	fmt.Printf("\nDP completed in %v\n", elapsed)
	fmt.Printf("Optimal total pages: %d (%d MB)\n", bestPages, bestPages*int64(pageSize)>>20)

	printStats(bestSlots, groups, pageAvail, pageSize)

	fmt.Printf("\nGo sizeClassSlotSize (without +24 slice header):\n")
	strs := make([]string, len(bestSlots))
	for i, s := range bestSlots {
		strs[i] = strconv.Itoa(s - *sliceHeader)
	}
	totalMB := bestPages * int64(pageSize) >> 20
	fmt.Printf("/*%dMB-%d-%dMB*/ %s,\n", (totalMB-int64(totalCount)*96>>20), len(bestSlots), totalMB, strings.Join(strs, ", "))

	fmt.Printf("\nGo sizeClassSlotSize (raw slot sizes including slice header):\n")
	strs2 := make([]string, len(bestSlots))
	for i, s := range bestSlots {
		strs2[i] = strconv.Itoa(s)
	}
	fmt.Printf("%s\n", strings.Join(strs2, ", "))
}

func loadCSV(filename string, sliceHeader, maxSlot int) []SizeGroup {
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
		if size > maxSlot {
			continue
		}
		size = (size + 7) &^ 7
		records = append(records, SizeGroup{Size: size, Count: count})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Size < records[j].Size
	})

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

func buildCandidates(groups []SizeGroup, pageAvail, maxSlot int) []int {
	set := make(map[int]bool)
	for _, g := range groups {
		set[g.Size] = true
	}
	for sz := 8; sz <= maxSlot; sz += 8 {
		set[sz] = true
	}
	for nslots := 1; nslots <= pageAvail/8; nslots++ {
		sz := pageAvail / nslots
		sz = sz &^ 7
		if sz >= 8 && sz <= maxSlot {
			set[sz] = true
		}
	}

	result := make([]int, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	sort.Ints(result)
	return result
}

// For a given minimum slot size, return the best candidate slot sizes to try.
// Prioritizes candidates that give more slots per page (good page utilization).
func topCandidatesForMinSlot(minSlot int, candidates []int, pageAvail, maxCand int) []int {
	startIdx := sort.SearchInts(candidates, minSlot)
	if startIdx >= len(candidates) {
		return nil
	}

	// For minimizing pages, the best slot is the smallest one that fits
	// (more slots per page = fewer pages). But sometimes a slightly larger
	// slot that's a perfect page divisor is better overall.
	//
	// Collect candidates up to a reasonable range above minSlot.
	maxCandSize := minSlot*3 + 256
	if maxCandSize > candidates[len(candidates)-1] {
		maxCandSize = candidates[len(candidates)-1]
	}

	type scored struct {
		slot  int
		pages int // slots per page (higher is better)
		tail  int // page tail waste (lower is better)
	}

	var scoredList []scored
	for ci := startIdx; ci < len(candidates); ci++ {
		c := candidates[ci]
		if c > maxCandSize {
			break
		}
		spp := pageAvail / c
		if spp == 0 {
			continue
		}
		tail := pageAvail - spp*c
		scoredList = append(scoredList, scored{slot: c, pages: spp, tail: tail})
	}

	// Sort by: slots per page descending, then tail waste ascending
	// This prioritizes candidates that pack more records per page.
	sort.Slice(scoredList, func(i, j int) bool {
		if scoredList[i].pages != scoredList[j].pages {
			return scoredList[i].pages > scoredList[j].pages
		}
		return scoredList[i].tail < scoredList[j].tail
	})

	// Also ensure we include the exact minSlot and a few perfect divisors
	limit := maxCand
	if limit > len(scoredList) {
		limit = len(scoredList)
	}

	result := make([]int, limit)
	for i := 0; i < limit; i++ {
		result[i] = scoredList[i].slot
	}
	sort.Ints(result)
	return result
}

func dpOptimize(groups []SizeGroup, candidates []int, K, pageAvail, maxSlot, maxCand int) ([]int, int64) {
	N := len(groups)

	// Prefix sums for O(1) range count queries
	prefixCount := make([]int64, N+1)
	for i, g := range groups {
		prefixCount[i+1] = prefixCount[i] + g.Count
	}

	rangeCount := func(from, to int) int64 {
		return prefixCount[to] - prefixCount[from]
	}

	// Cost = number of pages for slot size S covering groups[from..to)
	pagesForSlot := func(S int, from, to int) int64 {
		spp := int64(pageAvail / S)
		if spp == 0 {
			return math.MaxInt64 / 2
		}
		cnt := rangeCount(from, to)
		if cnt == 0 {
			return 0
		}
		return (cnt + spp - 1) / spp
	}

	// Precompute per-endpoint candidate lists
	endpointCands := make(map[int][]int)
	for _, g := range groups {
		if _, ok := endpointCands[g.Size]; !ok {
			endpointCands[g.Size] = topCandidatesForMinSlot(g.Size, candidates, pageAvail, maxCand)
		}
	}

	// For last class - maxSlot and nearby
	lastCands := []int{maxSlot}
	startIdx := sort.SearchInts(candidates, maxSlot)
	for ci := startIdx; ci < len(candidates) && ci < startIdx+10; ci++ {
		if candidates[ci] != maxSlot {
			lastCands = append(lastCands, candidates[ci])
		}
	}
	endpointCands[maxSlot] = lastCands

	const INF = int64(math.MaxInt64 / 2)

	prev := make([]int64, N+1)
	curr := make([]int64, N+1)
	for j := range prev {
		prev[j] = INF
	}
	prev[0] = 0

	type decision struct {
		splitAt  int
		slotSize int
	}
	decisions := make([][]decision, K+1)
	for k := range decisions {
		decisions[k] = make([]decision, N+1)
	}

	for k := 1; k <= K; k++ {
		for j := range curr {
			curr[j] = INF
		}

		t0 := time.Now()
		isLast := (k == K)

		for j := k; j <= N; j++ {
			if isLast && j != N {
				continue
			}

			var cands []int
			if isLast {
				cands = endpointCands[maxSlot]
			} else {
				cands = endpointCands[groups[j-1].Size]
			}

			bestW := INF
			bestI := -1
			bestS := 0

			for i := k - 1; i < j; i++ {
				if prev[i] >= INF {
					continue
				}

				for _, S := range cands {
					if isLast && S < maxSlot {
						continue
					}
					p := pagesForSlot(S, i, j)
					total := prev[i] + p
					if total < bestW {
						bestW = total
						bestI = i
						bestS = S
					}
				}
			}

			curr[j] = bestW
			decisions[k][j] = decision{splitAt: bestI, slotSize: bestS}
		}

		elapsed := time.Since(t0)
		if isLast {
			fmt.Printf("  DP class %d/%d: %.2fs -> total pages: %d\n",
				k, K, elapsed.Seconds(), curr[N])
		} else {
			minW := INF
			for _, w := range curr {
				if w < minW {
					minW = w
				}
			}
			fmt.Printf("  DP class %d/%d: %.2fs -> best partial pages: %d\n",
				k, K, elapsed.Seconds(), minW)
		}

		prev, curr = curr, prev
	}

	optimalPages := prev[N]

	slots := make([]int, K)
	j := N
	for k := K; k >= 1; k-- {
		d := decisions[k][j]
		slots[k-1] = d.slotSize
		j = d.splitAt
	}

	return slots, optimalPages
}

func printStats(slots []int, groups []SizeGroup, pageAvail, pageSize int) {
	sort.Ints(slots)
	var totalPages int64
	var totalCount int64
	var totalSlotWaste, totalTailWaste, totalLastPageWaste int64

	gi := 0
	fmt.Printf("\n%-6s %-10s %-12s %-10s %-8s %-12s %-12s %-12s\n",
		"Class", "SlotSize", "SlotsPerPg", "Records", "Pages", "SlotWaste", "TailWaste", "LastPgWaste")
	fmt.Println(strings.Repeat("-", 92))

	for si := 0; si < len(slots); si++ {
		slotSize := slots[si]
		slotsPerPage := pageAvail / slotSize
		if slotsPerPage == 0 {
			slotsPerPage = 1
		}
		pageTailWaste := pageAvail - slotsPerPage*slotSize

		var upperBound int
		if si < len(slots)-1 {
			upperBound = slots[si+1] - 1
		} else {
			upperBound = math.MaxInt32
		}

		var classCount int64
		var classSlotWaste int64
		for gi < len(groups) && groups[gi].Size <= upperBound {
			g := groups[gi]
			if g.Size > slotSize {
				break
			}
			classSlotWaste += int64(slotSize-g.Size) * g.Count
			classCount += g.Count
			gi++
		}

		var numPages int64
		var classTailWaste int64
		var classLastPageWaste int64
		if classCount > 0 {
			numPages = (classCount + int64(slotsPerPage) - 1) / int64(slotsPerPage)
			classTailWaste = numPages * int64(pageTailWaste)
			usedInLastPage := classCount % int64(slotsPerPage)
			if usedInLastPage == 0 {
				usedInLastPage = int64(slotsPerPage)
			}
			classLastPageWaste = (int64(slotsPerPage) - usedInLastPage) * int64(slotSize)
		}

		totalPages += numPages
		totalSlotWaste += classSlotWaste
		totalTailWaste += classTailWaste
		totalLastPageWaste += classLastPageWaste
		totalCount += classCount

		if classCount > 0 {
			fmt.Printf("%-6d %-10d %-12d %-10d %-8d %-12d %-12d %-12d\n",
				si, slotSize, slotsPerPage, classCount, numPages, classSlotWaste, classTailWaste, classLastPageWaste)
		}
	}

	fmt.Println(strings.Repeat("-", 92))
	fmt.Printf("Total records: %d\n", totalCount)
	fmt.Printf("Total pages:           %12d (%d MB)\n", totalPages, totalPages*int64(pageSize)>>20)
	fmt.Printf("Total slot waste:      %12d bytes (%.1f MB)\n", totalSlotWaste, float64(totalSlotWaste)/(1024*1024))
	fmt.Printf("Total tail waste:      %12d bytes (%.1f MB)\n", totalTailWaste, float64(totalTailWaste)/(1024*1024))
	fmt.Printf("Total last-page waste: %12d bytes (%.1f MB)\n", totalLastPageWaste, float64(totalLastPageWaste)/(1024*1024))
	totalWaste := totalSlotWaste + totalTailWaste + totalLastPageWaste
	fmt.Printf("Total waste:           %12d bytes (%.1f MB)\n", totalWaste, float64(totalWaste)/(1024*1024))
}
