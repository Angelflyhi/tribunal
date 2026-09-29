package judging

import (
	"math"
	"math/rand"
	"sort"
	"time"
)

// ZScoreNormalize standardizes a slice of raw scores for a single judge.
// It applies empirical shrinkage towards a prior (e.g. global mean/std)
// to prevent extreme variance blow-up when a judge has very few reviews.
func ZScoreNormalize(scores []float64, priorMean, priorStddev, shrinkage float64) []float64 {
	if len(scores) == 0 {
		return nil
	}
	if len(scores) == 1 {
		return []float64{0} // Can't estimate variance, return 0
	}

	sum := 0.0
	for _, s := range scores {
		sum += s
	}
	sampleMean := sum / float64(len(scores))

	sqSum := 0.0
	for _, s := range scores {
		sqSum += (s - sampleMean) * (s - sampleMean)
	}
	sampleVar := sqSum / float64(len(scores))
	sampleStd := math.Sqrt(sampleVar)

	// Shrinkage calculation:
	// alpha = n / (n + k), where k is the shrinkage pseudo-count (e.g. 3.0)
	// We blend the sample estimates with the prior estimates
	n := float64(len(scores))
	alpha := n / (n + shrinkage)

	shrunkMean := alpha*sampleMean + (1-alpha)*priorMean
	shrunkStd := alpha*sampleStd + (1-alpha)*priorStddev

	result := make([]float64, len(scores))
	if shrunkStd == 0 {
		return result // all 0s
	}

	for i, s := range scores {
		result[i] = (s - shrunkMean) / shrunkStd
	}
	return result
}

// JudgeDiscrimination estimates a judge's reliability by correlating their
// normalized scores with the consensus normalized scores.
func JudgeDiscrimination(judgeZ []float64, consensusZ []float64) float64 {
	if len(judgeZ) == 0 || len(judgeZ) != len(consensusZ) {
		return 0.1
	}

	// Calculate Pearson correlation
	sumX, sumY, sumXY, sumX2, sumY2 := 0.0, 0.0, 0.0, 0.0, 0.0
	n := float64(len(judgeZ))

	for i := 0; i < len(judgeZ); i++ {
		x := judgeZ[i]
		y := consensusZ[i]
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}

	numerator := (n * sumXY) - (sumX * sumY)
	denominator := math.Sqrt(((n * sumX2) - (sumX * sumX)) * ((n * sumY2) - (sumY * sumY)))

	if denominator == 0 {
		return 0.1
	}

	r := numerator / denominator
	if math.IsNaN(r) {
		r = 0.0
	}

	return math.Max(0.1, r)
}

type JudgeScore struct {
	Discrimination  float64
	NormalizedScore float64
}

// CalibratedScore computes a weighted aggregate score based on judge discrimination.
func CalibratedScore(judges []JudgeScore) float64 {
	num := 0.0
	den := 0.0
	for _, js := range judges {
		w := js.Discrimination
		z := js.NormalizedScore
		num += w * z
		den += w
	}
	if den == 0 {
		return 0
	}
	return num / den
}

// FitBradleyTerry uses the MM algorithm to estimate item qualities from pairwise comparison counts.
// wins[i][j] is the weighted number of times project i beat project j.
func FitBradleyTerry(wins [][]float64, n int) []float64 {
	pi := make([]float64, n)
	for i := range pi {
		pi[i] = 1.0
	}

	W := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			W[i] += wins[i][j]
		}
	}

	for iter := 0; iter < 200; iter++ {
		piNew := make([]float64, n)
		maxDelta := 0.0

		for i := 0; i < n; i++ {
			denom := 0.0
			for j := 0; j < n; j++ {
				if i == j {
					continue
				}
				nij := wins[i][j] + wins[j][i]
				if nij == 0 {
					continue
				}
				denom += nij / (pi[i] + pi[j])
			}
			if denom == 0 {
				piNew[i] = pi[i]
				continue
			}
			piNew[i] = W[i] / denom
		}

		// Normalize: sum = n
		total := 0.0
		for _, v := range piNew {
			total += v
		}
		if total > 0 {
			for i := range piNew {
				piNew[i] = piNew[i] * float64(n) / total
			}
		}

		// Check convergence
		for i := range pi {
			delta := math.Abs(piNew[i] - pi[i])
			if delta > maxDelta {
				maxDelta = delta
			}
		}

		copy(pi, piNew)

		if maxDelta < 1e-8 {
			break
		}
	}

	return pi
}

// IsComparisonGraphConnected checks if the comparison graph is strongly connected,
// a requirement for Bradley-Terry convergence.
func IsComparisonGraphConnected(wins [][]float64, n int) bool {
	if n == 0 {
		return true
	}
	visited := make([]bool, n)
	queue := []int{0}
	visited[0] = true
	count := 1

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		for j := 0; j < n; j++ {
			if !visited[j] && (wins[curr][j]+wins[j][curr]) > 0 {
				visited[j] = true
				count++
				queue = append(queue, j)
			}
		}
	}
	return count == n
}

type RankInterval struct {
	ProjectIndex int
	Lower        int
	Upper        int
}

// Basic struct and sort to facilitate ranks
type kv struct {
	index int
	val   float64
}

func computeRanks(scores []float64) []int {
	var kvs []kv
	for i, s := range scores {
		kvs = append(kvs, kv{i, s})
	}
	sort.Slice(kvs, func(i, j int) bool {
		return kvs[i].val > kvs[j].val // descending
	})
	ranks := make([]int, len(scores))
	for r, kv := range kvs {
		ranks[kv.index] = r + 1 // 1-based rank
	}
	return ranks
}

// BootstrapConfidenceIntervals estimates the 90% confidence bounds (5th and 95th percentiles)
// for the rank of each project by resampling the pairwise comparisons with replacement.
func BootstrapConfidenceIntervals(wins [][]float64, n int, iterations int) []RankInterval {
	if n == 0 {
		return nil
	}

	// Flatten all original comparisons into a list of (winner, loser) pairs
	var allComparisons [][2]int
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			count := int(math.Round(wins[i][j]))
			for k := 0; k < count; k++ {
				allComparisons = append(allComparisons, [2]int{i, j})
			}
		}
	}

	totalComps := len(allComparisons)
	if totalComps == 0 {
		intervals := make([]RankInterval, n)
		for i := 0; i < n; i++ {
			intervals[i] = RankInterval{ProjectIndex: i, Lower: 1, Upper: n}
		}
		return intervals
	}

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	projectRanks := make([][]int, n)

	for iter := 0; iter < iterations; iter++ {
		// Resample with replacement
		resampledWins := make([][]float64, n)
		for i := 0; i < n; i++ {
			resampledWins[i] = make([]float64, n)
		}

		for i := 0; i < totalComps; i++ {
			idx := rng.Intn(totalComps)
			pair := allComparisons[idx]
			resampledWins[pair[0]][pair[1]]++
		}

		// Calculate qualities and ranks for this bootstrap sample
		qualities := FitBradleyTerry(resampledWins, n)
		ranks := computeRanks(qualities)
		for i, r := range ranks {
			projectRanks[i] = append(projectRanks[i], r)
		}
	}

	var intervals []RankInterval
	for i := 0; i < n; i++ {
		sort.Ints(projectRanks[i])
		p5 := int(math.Floor(float64(iterations) * 0.05))
		p95 := int(math.Floor(float64(iterations) * 0.95))
		
		// Boundary checks
		if p95 >= iterations {
			p95 = iterations - 1
		}

		intervals = append(intervals, RankInterval{
			ProjectIndex: i,
			Lower:        projectRanks[i][p5],
			Upper:        projectRanks[i][p95],
		})
	}
	return intervals
}

// JudgeInfluence contains the max rank displacement caused by a judge.
type JudgeInfluence struct {
	JudgeID     string
	MaxRankDiff int
}

// LeaveOneOutJudgeInfluence estimates the robustness of the Bradley-Terry ranking
// by computing how much the final ranking changes if a specific judge's comparisons
// are removed.
func LeaveOneOutJudgeInfluence(wins [][]float64, n int, judgeComparisons map[string][][2]int) []JudgeInfluence {
	if n == 0 {
		return nil
	}

	// Base ranks
	baseQualities := FitBradleyTerry(wins, n)
	baseRanks := computeRanks(baseQualities)

	var influences []JudgeInfluence

	for judgeID, comparisons := range judgeComparisons {
		// Clone wins and remove this judge's comparisons
		looWins := make([][]float64, n)
		for i := 0; i < n; i++ {
			looWins[i] = make([]float64, n)
			copy(looWins[i], wins[i])
		}

		for _, comp := range comparisons {
			if looWins[comp[0]][comp[1]] > 0 {
				looWins[comp[0]][comp[1]]--
			}
		}

		looQualities := FitBradleyTerry(looWins, n)
		looRanks := computeRanks(looQualities)

		maxDiff := 0
		for i := 0; i < n; i++ {
			diff := baseRanks[i] - looRanks[i]
			if diff < 0 {
				diff = -diff
			}
			if diff > maxDiff {
				maxDiff = diff
			}
		}

		influences = append(influences, JudgeInfluence{
			JudgeID:     judgeID,
			MaxRankDiff: maxDiff,
		})
	}

	return influences
}

// EstimateIRTParameters computes a rough approximation of IRT Discrimination and Severity 
// for judges based on their pairwise comparisons, relative to a baseline consensus ranking.
func EstimateIRTParameters(judgeComparisons map[string][][2]int, consensusScores []float64, n int) (map[string]float64, map[string]float64) {
	discriminations := make(map[string]float64)
	severities := make(map[string]float64)

	for judgeID, comps := range judgeComparisons {
		if len(comps) == 0 {
			discriminations[judgeID] = 1.0
			severities[judgeID] = 0.0
			continue
		}

		var sumDiff float64
		var sumAbsDiff float64
		
		for _, comp := range comps {
			winner, loser := comp[0], comp[1]
			// The score difference between the winner and loser in the consensus ranking
			diff := consensusScores[winner] - consensusScores[loser]
			sumDiff += diff
			sumAbsDiff += math.Abs(diff)
		}

		// Discrimination: How well do their choices align with the consensus?
		// Positive if they pick the consensus winner, negative if they pick the consensus loser.
		var discrimination float64
		if sumAbsDiff > 0 {
			discrimination = sumDiff / sumAbsDiff
		} else {
			discrimination = 0.0
		}

		// Map to a weight [0.1, 2.0]
		weight := 1.0 + discrimination
		if weight < 0.1 {
			weight = 0.1
		}
		if weight > 2.0 {
			weight = 2.0
		}
		
		discriminations[judgeID] = weight
		severities[judgeID] = 0.0 // Severity doesn't easily translate to unanchored pairwise comparisons
	}

	return discriminations, severities
}
