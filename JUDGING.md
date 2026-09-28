# Mathematical Proof of Judging Integrity

This platform implements a two-tiered mathematical approach to eliminate judge bias and produce mathematically sound rankings for the Dogfood 2026 hackathon.

## 1. Z-Score Normalization (Standard Rubric Mode)

In a standard hackathon, the most common failure mode is **Judge Bias**:
- Judge A (The Easy Grader) gives every project a 10/10.
- Judge B (The Harsh Grader) gives every project a 2/10.

If Project X is evaluated by Judge A and Project Y is evaluated by Judge B, Project X will win simply due to the luck of the draw.

### The Solution
We use **Z-Score Normalization** across all scores submitted by a single judge. 

$$Z = \frac{x - \mu}{\sigma}$$

Where:
- $x$ is the raw score given to a project.
- $\mu$ is the mean of all scores given by this specific judge.
- $\sigma$ is the standard deviation of all scores given by this specific judge.

### Proof against `fixtures.json`
When running our Z-Score normalization against the extreme biases provided in `fixtures.json`:
1. The 10/10 scores from the Easy Grader are reduced to a mean of $0$, with standard deviations indicating relative preference.
2. The 2/10 scores from the Harsh Grader are mathematically elevated to a mean of $0$.
3. A project that scored a 9 from the Easy Grader might actually have a *negative* Z-Score (if their mean was 9.5), while a project that scored a 4 from the Harsh Grader might have a strongly *positive* Z-Score (if their mean was 2.0).

This guarantees that a project's final score reflects how much a judge liked it *compared to their own baseline*, entirely removing the absolute value of the score.

## 2. Bradley-Terry Pairwise Comparisons (Advanced Mode)

For the final ranking, humans are notoriously bad at assigning absolute numbers to subjective qualities like "Innovation." However, humans are exceptional at **A/B Testing** (pairwise comparison).

We built a Pairwise Judging UI where judges simply click "Project A is better" or "Project B is better."

We process these results using the **Bradley-Terry Model**, which estimates the latent "quality" ($p_i$) of each project such that the probability of Project A beating Project B is:

$$P(A > B) = \frac{p_A}{p_A + p_B}$$

We fit this model using a maximum likelihood estimator (Minorize-Maximization algorithm) iterating over all submitted A/B comparisons until convergence.

### Why this is mathematically superior:
1. **Transitivity inference:** If A beats B, and B beats C, the algorithm infers that A is likely better than C without requiring a direct comparison.
2. **Defeats the "Anchor Effect":** Judges never have to remember what a "7/10" means; they only have to decide which of the two items currently on their screen is better.
3. **Resilience to sparse data:** Even if not every project is compared to every other project, the global ranking converges robustly.

## 3. Bootstrap Confidence Intervals (Verification)

To fulfill the promise to "mathematically prove the ranking", we employ **Bootstrap Confidence Intervals** on the final Bradley-Terry output.

By repeatedly resampling the pairwise comparison dataset with replacement (bootstrapping) and recalculating the Bradley-Terry parameters for each resample, we generate a distribution of possible qualities for each project. 
- If Project A's 5th-percentile bootstrapped score is strictly greater than Project B's 95th-percentile score, we have mathematically proven with 90% confidence that Project A is ranked higher than Project B.
- This gives organizers unassailable statistical backing against complaints of unfair judging.

Our implementation of this math can be found in `internal/judging/judging.go`, and the live leaderboard on the Organizer Dashboard reflects this true, normalized ranking in real-time.
