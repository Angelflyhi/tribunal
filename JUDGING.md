# Judging Architecture: Beyond 5-Star Ratings

Traditional 1-5 star rubric judging fails at scale because of **judge bias** (some judges are inherently strict, some are lenient) and **anchoring** (scores shift based on the project seen right before). 

Tribunal solves this using the **Bradley-Terry Pairwise Model**.

## The Mechanism

Instead of rating a project from 1-5, judges are presented with two projects simultaneously and asked: *"Which project is better?"*

This forms a directed graph of wins and losses. We use the **Bradley-Terry Model** to extract a global, objective ranking from these sparse head-to-head collisions.

### The Algorithm
For any two projects $i$ and $j$, the model states the probability that $i$ beats $j$ is:
$$ P(i \text{ beats } j) = \frac{p_i}{p_i + p_j} $$
where $p_i$ is the true underlying skill (score) of project $i$.

Tribunal uses an iterative Maximum Likelihood Estimation (MM algorithm) to compute $p_i$ for all projects.
We also apply empirical Bayes shrinkage (adding pseudo-comparisons) to prevent projects with a 100% win rate from reaching infinite scores.

### Adaptive Pairing
Tribunal does not assign comparisons randomly. It tracks the variance and match counts for every project, assigning pairs that maximize information gain (projects with similar uncertain scores are matched together).

## Defensibility (Leave-One-Out)

A critical requirement for high-stakes hackathons is determining if the leaderboard is resilient. 
Tribunal calculates **Judge Influence** using Leave-One-Out (LOO) analysis:
1. Re-run the entire Bradley-Terry estimation, but remove the votes of Judge X.
2. Calculate how many ranks the top projects shifted.
3. If removing a single judge drastically alters the top 3, the leaderboard is flagged as **"High Risk"** in the Organizer Dashboard, prompting the organizer to seek more comparisons.
