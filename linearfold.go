package linearfold

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
	"strings"
)

/******************************************************************************
May 12, 2021

LinearFold is an algorithm developed in 2019 that can simulate the folding of
RNA in linear-time rather than cubic time, which is what many algorithms use.

Paper here:
https://doi.org/10.1093/bioinformatics/btz375

There are 2 novel ideas that let LinearFold get developed:

1. Sequences are folded left to right (5'->3'). This would naively increase
   the algorithm from O(n^3) to O(3^n), but they borrow an efficient packing
   algorithm that reduces back to O(n^3)
2. The algorithm uses a beam search while folding sequences to reduce the search
   space from O(n^3) to O(b*n*log(b)) where b is the beam size


## Explanation of algorithm

Figure 2 from the LinearFold paper may be of assistance here.

To understand the full algorithm, let's begin with a naive implementation. There
are 3 elements that are managed: The structure, the score, and the stack.

1. The structure is the structure of the RNA molecule. It is reprsented in paratheses
   format, ie: ((.))
2. The score is a float the current score of any given RNA fold. The score is calculated from
   the paratheses format.
3. The stack an integer number of parentheses without matches. For example, a molecule of
   structure ((.) would have a stack of 1.

As you are iterating along an RNA sequence from left to right, you can do 3 actions on the
current structure: push by adding a '(', skip by adding a '.', or pop by adding a ')'. These
actions affect the stack and influence the score.

Sequence: CCAGG

sequence:  C -> CC -> CCA -> CCAG -> CCAGG
structure: (    ((    ((.    ((.)    ((.))
score:     0    0     0      +0.9    +1.9
stack:     1    2     2      1       0

During each step where you add a nucleotide, explore each combination of a push, skip, or pop (3^n).
At the end of the sequence, return the structure with the highest score. Obviously, this algorithm
explodes in complexity, so let's make it a little simpler.



### Idea 1: Merge states: O(3^n) -> O(2^n)

First, let's define the possible current state during a step

type State struct {
	Structure string
	Score     float64
	Stack     int
}

If you have 2 States with an equivalent stack at an equivalent step, for example:

state1 := State{"((.))", 1.9, 0}
state2 := State{".(.).", 0.7, 0}

You can "merge" both states together, since the RNA will fold to state1 rather than
state2. You no longer need to pay attention to state2, since state1 is more favorable.
All future skips, pushes and pops will be off of state1, not state2.

### Idea 2: Stack state heads: O(2^n) -> O(n^3)

Imagine we have 2 states at an equivalent step, but they do not have an equivalent stack value.
For example:

state1 := State{"(", 0.0, 1}
state2 := State{".", -0.1, 0}

If you add a skip or a push [. or (], both of these states will act the same (ie,
they'll keep a constant score). Only when you pop, or add a ")", will they diverge.
To save space, we can temporarily pack both states together when doing a skip or a push.

For example, "(" and ".", when adding a "(", could become "?(". This "?(" you
can add onto until you do a pop, where you will have to unpack the previous values and evaluate them.

This is best demonstrated in Fig2B of the paper.

### Idea 3: Prune states: O(n^3) -> O(n)

This is very simple. As we're searching along all possible structures, prune
the ones that work the worst. Our search that moves from left to right or 5'->3'
fits this model quite nicely, since RNA polymerase actually works like this.

******************************************************************************/

// Magic numbers
// Should add source and small description of what the number is about
var external_unpaired float64 = -0.009729
var external_paired float64 = -0.000967
var multi_unpaired float64 = -0.198330
var multi_paired float64 = -0.925388
var multi_base float64 = -1.199055

var bulge_length = [31]float64{0.0, -2.399548472, -3.2940667837, -4.2029218746, -5.0441693501, -5.4807172844, -6.0506360645, -5.8503526421, -5.0964765063, -5.7009810518, -6.4210758616, -6.9347480537, -7.2962207216, -7.5576661608, -7.7170588501, -7.80330553291, -7.83437644287, -7.84534866319, -7.81533646036, -7.76774522247, -7.81070694312, -7.82862593974, -7.90663145496, -7.97762471926, -8.03530424822, -8.08164219503, -8.11723639959, -8.14398574353, -8.16217532325, -8.17269833057, -8.17785195742}
var internal_length = [31]float64{0.0, 0.0, -0.429061443, -0.7822725931, -1.1786523466, -1.4897722641, -1.7449668113, -1.79645798028, -1.83964800435, -1.83766251486, -2.01381382847, -2.27778244917, -2.62384380687, -2.91650411476, -2.95274661783, -3.07274199393, -3.11628971319, -3.19838264454, -3.20549587058, -3.18194762206, -3.15127788635, -3.21746029729, -3.34906953559, -3.48986908699, -3.55587200561, -3.63366405305, -3.6845060657, -3.72590482171, -3.72262823831, -3.71670365547, -3.70982791746}
var internal_explicit = [21]float64{0.0, 0.0, 0.0, 0.0, 0.0, -0.1754591076, 0.03083787104, -0.171565435, -0.2294680983, 0.0, -0.1304072693, -0.07730329553, 0.2782767264, 0.0, 0.0, -0.02898949617, 0.3112350694, 0.0, 0.0, 0.0, -0.3226348245}
var internal_symmetric_length = [16]float64{0.0, -0.5467082599, -0.9321784246, -1.1910250647, -1.4251087392, -1.2800509627, -1.9363442142, -2.2384530511, -2.26877580377, -2.62057020957, -2.83648346017, -2.95931050557, -3.11453136507, -3.1999425725, -3.24586367049, -3.26818601285}
var internal_asymmetry = [29]float64{0.0, -2.105646719, -2.6576607621, -3.2347315291, -3.8483983138, -4.1541139979, -4.269619198, -4.4801804211, -4.7947547341, -5.1096509022, -5.19983279712, -5.41983547652, -5.56048380082, -5.77672492672, -5.94927807022, -6.10516925682, -6.20925512312, -6.2789319654, -6.31999174034, -6.3356979835, -6.32187797711, -6.28055809148, -6.24461623198, -6.21639436916, -6.20002851042, -6.17452794867, -6.14104762074, -6.10132837662, -6.10387349055}
var hairpin_length = [31]float64{-5.993180158, -9.10128592, -8.6843882853, -6.4789692193, -4.5522195273, -5.1395440602, -5.222301238, -4.6439122536, -5.3660005908, -5.5385880532, -5.8410970399, -5.8707286338, -6.7976282286, -6.82920576838, -6.93145297848, -6.74131224388, -6.83412134214, -6.66507650134, -6.74680216605, -7.09139606915, -7.20054636315, -7.49089873245, -7.83027009915, -8.02180651085, -8.07199860464, -8.11074481388, -8.06323010636, -7.9957868871, -7.89856812984, -7.73125495654, -7.49826123164}
var terminal_mismatch = [625]float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.184546064, -0.1181844187, -0.4461469607, -0.6175254495, 0.0, 0.004788458708, 0.08319395146, -0.2249479995, -0.3981327204, 0.0, 0.5191110288, -0.3524119307, -0.4056429433, -0.7733932162, 0.0, -0.01574403519, 0.268570042, -0.0934388741, 0.3373711531, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.08386423535, -0.2520716816, -0.6711841881, -0.3816350028, 0.0, 0.1117852189, -0.1704393624, -0.2179987732, -0.459267635, 0.0, 0.8520640313, -0.9332488517, -0.3289551692, -0.7778822056, 0.0, -0.2422339958, -0.03780509247, -0.4322334143, -0.2419976114, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.1703136025, -0.09154056357, -0.2522413002, -0.8520314799, 0.0, 0.04763224188, -0.2428654283, -0.2079275061, -0.1874270053, 0.0, 0.6540033983, -0.7823988605, 0.1995898255, -0.4432169392, 0.0, -0.1736921762, 0.288494362, -0.01638238057, 0.6757988971, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.4871607613, 0.1105031953, 0.363373916, -0.6193199348, 0.0, 0.3451056056, 0.0314944976, -0.3799172956, -0.03222973182, 0.0, 0.4948638637, -0.2821952552, -0.2702227211, -0.06658395291, 0.0, -0.4306154451, -0.09497863465, -0.3130794485, -0.2283242981, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0115363879, -0.3923408221, 0.05661063599, -0.1251485388, 0.0, -0.06545074758, -0.3167200568, 0.002258383981, -0.422217724, 0.0, 0.5458416646, -0.2085887954, -0.1971766062, -0.4722410132, 0.0, -0.1779642496, 0.1643454344, -0.5005617032, 0.1333867679, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.1218741278, 0.1990260141, 0.04681893928, 0.3256264491, 0.0, 0.1186812326, -0.1851065102, -0.04311512683, -0.6150608139, 0.0, 0.754933218, -0.3150708483, 0.1569582926, -0.514970007, 0.0, -0.2926246029, 0.1373068149, -0.05422333363, 0.03086776921, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
var helix_closing = [25]float64{0.0, 0.0, 0.0, -0.9770893163, 0.0, 0.0, 0.0, -0.4574650937, 0.0, 0.0, 0.0, -0.8265995623, 0.0, -1.051678928, 0.0, -0.9246140521, 0.0, -0.3698708172, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
var base_pair = [25]float64{0.0, 0.0, 0.0, 0.59791199, 0.0, 0.0, 0.0, 1.544290641, 0.0, 0.0, 0.0, 1.544290641, 0.0, -0.01304754992, 0.0, 0.59791199, 0.0, -0.01304754992, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
var dangle_left = [125]float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.1251037681, 0.0441606708, -0.02541879082, 0.00785098466, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.07224381372, 0.05279281874, 0.1009554299, -0.1515059013, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.1829535099, 0.03393000394, 0.1335339061, -0.1604274506, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.06517511341, -0.04250882422, 0.02875971806, -0.04359727428, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.03373847659, -0.005070324324, -0.1186861149, -0.01162357727, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.08047139148, 0.001608000669, 0.1016272216, -0.09200842832, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
var dangle_right = [125]float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.03232578201, -0.09096819493, -0.0740750973, -0.01621157379, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.2133964379, -0.06234810991, -0.07008531041, -0.2141912285, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.01581957549, 0.005644320058, -0.00943297687, -0.2597793095, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.04480271781, -0.07321213002, 0.01270494867, -0.05717033985, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.1631918513, 0.06769304994, -0.08789074414, -0.05525570007, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.04105458185, -0.008136642572, -0.03808592022, -0.08629373429, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
var helix_stacking = [625]float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.1482005248, 0.0, 0.0, 0.0, 0.4343497127, 0.0, 0.0, 0.0, 0.7079642577, 0.0, -0.1010777582, 0.0, 0.243256656, 0.0, 0.1623654243, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.4878707793, 0.0, 0.0, 0.0, 0.8481320247, 0.0, 0.0, 0.0, 0.4784248478, 0.0, -0.1811268205, 0.0, 0.7079642577, 0.0, 0.4849351028, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.5551785831, 0.0, 0.0, 0.0, 0.5008324248, 0.0, 0.0, 0.0, 0.8481320247, 0.0, 0.2165962476, 0.0, 0.4343497127, 0.0, 0.4864603589, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.04665365028, 0.0, 0.0, 0.0, 0.4864603589, 0.0, 0.0, 0.0, 0.4849351028, 0.0, 0.1833447295, 0.0, 0.1623654243, 0.0, -0.2858970755, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.3897593783, 0.0, 0.0, 0.0, 0.5551785831, 0.0, 0.0, 0.0, 0.4878707793, 0.0, -0.1157333764, 0.0, 0.1482005248, 0.0, -0.04665365028, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, -0.1157333764, 0.0, 0.0, 0.0, 0.2165962476, 0.0, 0.0, 0.0, -0.1811268205, 0.0, 0.120296538, 0.0, -0.1010777582, 0.0, 0.1833447295, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
var bulge_0x1_nucleotides = [5]float64{-0.1216861662, -0.07111241127, 0.008947026647, -0.002685763742, 0.0}
var internal_1x1_nucleotides = [25]float64{0.2944404686, 0.08641360967, -0.3664197228, -0.2053107048, 0.0, 0.08641360967, -0.1582543624, 0.4175273724, 0.1368762582, 0.0, -0.3664197228, 0.4175273724, -0.1193514754, -0.4188101413, 0.0, -0.2053107048, 0.1368762582, -0.4188101413, 0.147140653, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}

const NOTON int = 5 // NUM_OF_TYPE_OF_NUCS
const NOTOND int = 25
const NOTONT int = 125

const SINGLE_MAX_LEN int = 30 // NOTE: *must* <= sizeof(char), otherwise modify State::TraceInfo accordingly
const SINGLE_MIN_LEN int = 0
const MIN_CUBE_PRUNING_SIZE int = 20

var INTERNAL_MAX_LEN int = SINGLE_MAX_LEN

const HAIRPIN_MAX_LEN int = 30
const EXPLICIT_MAX_LEN int = 4
const SYMMETRIC_MAX_LEN int = 15
const ASYMMETRY_MAX_LEN int = 28

// recommended beam size based on paper
var beam_size int = 100

// (((((((..((((.......))))(((((((.....))))))).(((((.......))))))))))))....

var VALUE_MIN float64 = -math.MaxFloat64

// Can we remove these or init them here?
var _allowed_pairs [NOTON][NOTON]bool
var _helix_stacking [NOTON][NOTON][NOTON][NOTON]bool
var cache_single [SINGLE_MAX_LEN + 1][SINGLE_MAX_LEN + 1]float64
var scores []Pair
var bestC []*State
var bestH [](map[int]*State)
var bestP [](map[int]*State)
var bestM [](map[int]*State)
var bestM2 [](map[int]*State)
var bestMulti [](map[int]*State)
var sorted_bestM [][]Pair

func GET_ACGU_NUM(x rune) int {
	switch x {
	case 'A':
		return 0
	case 'C':
		return 1
	case 'G':
		return 2
	case 'U':
		return 3
	default:
		return 4
	}
}

func initialize() {
	_allowed_pairs[GET_ACGU_NUM('A')][GET_ACGU_NUM('U')] = true
	_allowed_pairs[GET_ACGU_NUM('U')][GET_ACGU_NUM('A')] = true
	_allowed_pairs[GET_ACGU_NUM('C')][GET_ACGU_NUM('G')] = true
	_allowed_pairs[GET_ACGU_NUM('G')][GET_ACGU_NUM('C')] = true
	_allowed_pairs[GET_ACGU_NUM('G')][GET_ACGU_NUM('U')] = true
	_allowed_pairs[GET_ACGU_NUM('U')][GET_ACGU_NUM('G')] = true

	SET_HELIX_STACKING('A', 'U', 'A', 'U', true)
	SET_HELIX_STACKING('A', 'U', 'C', 'G', true)
	SET_HELIX_STACKING('A', 'U', 'G', 'C', true)
	SET_HELIX_STACKING('A', 'U', 'G', 'U', true)
	SET_HELIX_STACKING('A', 'U', 'U', 'A', true)
	SET_HELIX_STACKING('A', 'U', 'U', 'G', true)
	SET_HELIX_STACKING('C', 'G', 'A', 'U', true)
	SET_HELIX_STACKING('C', 'G', 'C', 'G', true)
	SET_HELIX_STACKING('C', 'G', 'G', 'C', true)
	SET_HELIX_STACKING('C', 'G', 'G', 'U', true)
	SET_HELIX_STACKING('C', 'G', 'U', 'G', true)
	SET_HELIX_STACKING('G', 'C', 'A', 'U', true)
	SET_HELIX_STACKING('G', 'C', 'C', 'G', true)
	SET_HELIX_STACKING('G', 'C', 'G', 'U', true)
	SET_HELIX_STACKING('G', 'C', 'U', 'G', true)
	SET_HELIX_STACKING('G', 'U', 'A', 'U', true)
	SET_HELIX_STACKING('G', 'U', 'G', 'U', true)
	SET_HELIX_STACKING('G', 'U', 'U', 'G', true)
	SET_HELIX_STACKING('U', 'A', 'A', 'U', true)
	SET_HELIX_STACKING('U', 'A', 'G', 'U', true)
	SET_HELIX_STACKING('U', 'G', 'G', 'U', true)
}

func SET_HELIX_STACKING(x rune, y rune, z rune, w rune, val bool) {
	_helix_stacking[GET_ACGU_NUM(x)][GET_ACGU_NUM(y)][GET_ACGU_NUM(z)][GET_ACGU_NUM(w)] = val
}

func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func initialize_cachesingle() {
	for i := 0; i < SINGLE_MAX_LEN+1; i++ {
		for j := 0; j < SINGLE_MAX_LEN+1; j++ {
			cache_single[i][j] = 0
		}
	}

	for l1 := SINGLE_MIN_LEN; l1 <= SINGLE_MAX_LEN; l1++ {
		for l2 := SINGLE_MIN_LEN; l2 <= SINGLE_MAX_LEN; l2++ {
			if l1 == 0 && l2 == 0 {
				continue
			} else if l1 == 0 {
				cache_single[l1][l2] += bulge_length[l2]
			} else if l2 == 0 {
				cache_single[l1][l2] += bulge_length[l1]
			} else {
				// internal
				cache_single[l1][l2] += internal_length[Min(l1+l2, INTERNAL_MAX_LEN)]

				// internal explicit
				if l1 <= EXPLICIT_MAX_LEN && l2 <= EXPLICIT_MAX_LEN {
					if l1 <= l2 {
						cache_single[l1][l2] += internal_explicit[l1*EXPLICIT_MAX_LEN+l2]
					} else {
						cache_single[l1][l2] += internal_explicit[l2*EXPLICIT_MAX_LEN+l1]
					}
				}

				if l1 == l2 {
					// internal symmetry
					cache_single[l1][l2] += internal_symmetric_length[Min(l1, SYMMETRIC_MAX_LEN)]
				} else {
					// internal asymmetry
					var diff int = l1 - l2
					if diff < 0 {
						diff = -diff
					}
					cache_single[l1][l2] += internal_asymmetry[Min(diff, ASYMMETRY_MAX_LEN)]
				}
			}

		}
	}
}

// What is manner?
const (
	MANNER_NONE                  = iota // 0: empty
	MANNER_H                            // 1: hairpin candidate
	MANNER_HAIRPIN                      // 2: hairpin
	MANNER_SINGLE                       // 3: single
	MANNER_HELIX                        // 4: helix
	MANNER_MULTI                        // 5: multi = ..M2. [30 restriction on the left and jump on the right]
	MANNER_MULTI_eq_MULTI_plus_U        // 6: multi = multi + U
	MANNER_P_eq_MULTI                   // 7: P = (multi)
	MANNER_M2_eq_M_plus_P               // 8: M2 = M + P
	MANNER_M_eq_M2                      // 9: M = M2
	MANNER_M_eq_M_plus_U                // 10: M = M + U
	MANNER_M_eq_P                       // 11: M = P
	/* MANNER_C_eq_U, */
	/* MANNER_C_eq_P, */
	MANNER_C_eq_C_plus_U // 12: C = C + U
	MANNER_C_eq_C_plus_P // 13: C = C + P
)

//
type State struct {
	score float64
	// age    int
	manner int
	// trace is a union in the original code. Need to be aware of this!
	trace struct {
		split    int
		paddings struct {
			l1 rune
			l2 int
		}
	}
}

func NewState(score float64, manner int) *State {
	return &State{score: score, manner: manner}
}

// func NewState() *State {
// 	return &State{score: math.MinFloat64, manner: MANNER_NONE}
// }

func update_if_better(s *State, newscore float64, manner int) {
	if s.score < newscore {
		s.Set(newscore, manner)
	}
}
func update_if_better2(s *State, newscore float64, manner int, l1 rune, l2 int) {
	if s.score < newscore || s.manner == MANNER_NONE {
		s.Set2(newscore, manner, l1, l2)
	}
}

func update_if_better3(s *State, newscore float64, manner int, split int) {
	if s.score < newscore || s.manner == MANNER_NONE {
		// ++ nos_set_update;
		s.Set3(newscore, manner, split)
	}
}

func (s *State) Set(score float64, manner int) {
	s.score = score
	s.manner = manner
}

func (s *State) Set2(score float64, manner int, l1 rune, l2 int) {
	s.score = score
	s.manner = manner
	s.trace.paddings.l1 = l1
	s.trace.paddings.l2 = l2
}

func (s *State) Set3(score float64, manner int, split int) {
	s.score = score
	s.manner = manner
	s.trace.split = split
}

// Assumes valid sequence input
// Output: Linearfold score, structure
func LinearFold(sequence string) (string, float64) {
	initialize()
	initialize_cachesingle()

	// convert to uppercase
	sequence = strings.ToUpper(sequence)

	// convert T to U
	sequence = strings.Replace(sequence, "T", "U", -1)

	// lhuang: moved inside loop, fixing an obscure but crucial bug in initialization
	// BeamCKYParser parser(beam_size);
	// BeamCKYParser::DecoderResult result = parser.parse(seq, NULL);
	return Parse(sequence)
}

type Pair struct {
	first, second interface{}
}

// ByFirst implements sort.Interface for []Pair based on
// the First field.
// type ByFirst []Pair

// func (a ByFirst) Len() int           { return len(a) }
// func (a ByFirst) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
// func (a ByFirst) Less(i, j int) bool { return a[i].first < a[j].first }

// An IntHeap is a min-heap of ints.
type PairHeap []Pair

func (h PairHeap) Len() int           { return len(h) }
func (h PairHeap) Less(i, j int) bool { return h[i].first.(float64) < h[j].first.(float64) }
func (h PairHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *PairHeap) Push(x interface{}) {
	// Push and Pop use pointer receivers because they modify the slice's length,
	// not just its contents.
	*h = append(*h, x.(Pair))
}

func (h *PairHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[0 : n-1]
	return x
}

func Parse(sequence string) (string, float64) {

	// number of states
	var nos_H, nos_P, nos_M2, nos_M, nos_C, nos_Multi uint64 = 0, 0, 0, 0, 0, 0

	seq_length := len(sequence)

	nucs := make([]int, seq_length)

	bestC = make([]*State, seq_length)
	for i := 0; i < seq_length; i++ {
		bestC[i] = NewState(VALUE_MIN, MANNER_NONE)
	}
	bestH = make([](map[int]*State), seq_length)
	bestP = make([](map[int]*State), seq_length)
	bestM = make([](map[int]*State), seq_length)
	bestM2 = make([](map[int]*State), seq_length)
	bestMulti = make([](map[int]*State), seq_length)
	for i := 0; i < seq_length; i++ {
		bestH[i] = make(map[int]*State)
		bestP[i] = make(map[int]*State)
		bestM[i] = make(map[int]*State)
		bestM2[i] = make(map[int]*State)
		bestMulti[i] = make(map[int]*State)
	}

	sorted_bestM = make([][]Pair, seq_length)

	// vector to store the scores at each beam temporarily for beam pruning
	scores = make([]Pair, seq_length)

	// Convert from ACGU to integers
	for i := 0; i < seq_length; i++ {
		nucs[i] = GET_ACGU_NUM((rune)(sequence[i]))
	}

	// What is next_pair?
	// int *next_pair = new int[NOTON * seq_length]{-1};
	next_pair := make([][]int, NOTON)

	// Iterate through ACGU
	for nuci := 0; nuci < NOTON; nuci++ {
		next_pair[nuci] = make([]int, seq_length)
		for j := 0; j < seq_length; j++ {
			next_pair[nuci][j] = -1
		}

		next := -1
		for j := seq_length - 1; j >= 0; j-- {
			next_pair[nuci][j] = next
			if _allowed_pairs[nuci][nucs[j]] {
				next = j
			}
		}
	}

	if seq_length > 0 {
		bestC[0].Set(ScoreExternalUnpaired(0, 0), MANNER_C_eq_C_plus_U)
	}

	if seq_length > 1 {
		bestC[1].Set(ScoreExternalUnpaired(0, 1), MANNER_C_eq_C_plus_U)
	}

	nos_C++

	// from left to right
	for j := 0; j < seq_length; j++ {
		nucj := nucs[j]
		nucj1 := 0
		if (j + 1) < seq_length {
			nucj1 = nucs[j+1]
		} else {
			nucj1 = -1
		}

		var beamstepH *map[int]*State = &bestH[j]
		var beamstepMulti *map[int]*State = &bestMulti[j]
		var beamstepP *map[int]*State = &bestP[j]
		var beamstepM2 *map[int]*State = &bestM2[j]
		var beamstepM *map[int]*State = &bestM[j]
		var beamstepC *State = bestC[j]

		// beam of H
		{
			if beam_size > 0 && len(*beamstepH) > beam_size {
				BeamPrune(beamstepH)
			}

			{
				// for nucj put H(j, j_next) into H[j_next]
				jnext := next_pair[nucj][j]

				for jnext-j < 4 && jnext != -1 {
					jnext = next_pair[nucj][jnext]
				}

				if jnext != -1 {
					nucjnext := nucs[jnext]
					var nucjnext_1 int
					if (jnext - 1) > -1 {
						nucjnext_1 = nucs[jnext-1]
					} else {
						nucjnext_1 = -1
					}
					var newscore float64
					newscore = score_hairpin(j, jnext, nucj, nucj1, nucjnext_1, nucjnext)

					// this candidate must be the best one at [j, jnext]
					// so no need to check the score
					if bestH[jnext][j] == nil {
						bestH[jnext][j] = NewState(VALUE_MIN, MANNER_NONE)
					}
					// fmt.Printf("Going to try to update bestH. jnext = %v, j = %v\n", jnext, j)
					// fmt.Printf("newscore: %v\n", newscore)
					update_if_better(bestH[jnext][j], newscore, MANNER_H)
					// fmt.Printf("Score after update: %v\n", bestH[jnext][j].score)
					nos_H++
				}
			}

			{
				// for every state h in H[j]
				//   1. extend h(i, j) to h(i, jnext)
				//   2. generate p(i, j)
				for i, state := range *beamstepH {
					nuci := nucs[i]
					jnext := next_pair[nuci][j]

					// 2. generate p(i, j)
					// lisiz, change the order because of the constriants
					{
						if (*beamstepP)[i] == nil {
							(*beamstepP)[i] = NewState(VALUE_MIN, MANNER_NONE)
						}
						update_if_better((*beamstepP)[i], state.score, MANNER_HAIRPIN)
						nos_P++
					}

					if jnext != -1 {
						var nuci1 int
						if (i + 1) < seq_length {
							nuci1 = nucs[i+1]
						} else {
							nuci1 = -1
						}

						nucjnext := nucs[jnext]

						var nucjnext_1 int
						if (jnext - 1) > -1 {
							nucjnext_1 = nucs[jnext-1]
						} else {
							nucjnext_1 = -1
						}

						// 1. extend h(i, j) to h(i, jnext)
						var newscore float64

						newscore = score_hairpin(i, jnext, nuci, nuci1, nucjnext_1, nucjnext)
						// this candidate must be the best one at [i, jnext]
						// so no need to check the score

						if bestH[jnext][i] == nil {
							bestH[jnext][i] = NewState(VALUE_MIN, MANNER_NONE)
						}
						update_if_better(bestH[jnext][i], newscore, MANNER_H)
						nos_H++
					}
				}
			}
		}

		if j == 0 {
			continue
		}

		// beam of Multi
		{
			if beam_size > 0 && len(*beamstepMulti) > beam_size {
				BeamPrune(beamstepMulti)
			}

			// for every state in Multi[j]
			//   1. extend (i, j) to (i, jnext)
			//   2. generate P (i, j)
			for i, state := range *beamstepMulti {

				nuci := nucs[i]
				nuci1 := nucs[i+1]
				jnext := next_pair[nuci][j]

				// 2. generate P (i, j)
				// lisiz, change the order because of the constraits
				{
					var newscore float64
					newscore = state.score + score_multi(i, j, nuci, nuci1, nucs[j-1], nucj, seq_length)
					if (*beamstepP)[i] == nil {
						(*beamstepP)[i] = NewState(VALUE_MIN, MANNER_NONE)
					}
					update_if_better((*beamstepP)[i], newscore, MANNER_P_eq_MULTI)
					nos_P++
				}

				// 1. extend (i, j) to (i, jnext)
				{
					new_l1 := state.trace.paddings.l1
					new_l2 := state.trace.paddings.l2 + jnext - j
					// if (jnext != -1 && new_l1 + new_l2 <= SINGLE_MAX_LEN) {
					if jnext != -1 {
						// 1. extend (i, j) to (i, jnext)
						newscore := state.score + score_multi_unpaired(j, jnext-1)
						// this candidate must be the best one at [i, jnext]
						// so no need to check the score
						if bestMulti[jnext][i] == nil {
							bestMulti[jnext][i] = NewState(VALUE_MIN, MANNER_NONE)
						}
						update_if_better2(bestMulti[jnext][i], newscore, MANNER_MULTI_eq_MULTI_plus_U,
							new_l1,
							new_l2)
						nos_Multi++
					}
				}
			}
		}

		// beam of P
		{
			if beam_size > 0 && len(*beamstepP) > beam_size {
				BeamPrune(beamstepP)
			}

			// for every state in P[j]
			//   1. generate new helix/bulge
			//   2. M = P
			//   3. M2 = M + P
			//   4. C = C + P
			var use_cube_pruning bool = beam_size > MIN_CUBE_PRUNING_SIZE && len(*beamstepP) > MIN_CUBE_PRUNING_SIZE

			for i, state := range *beamstepP {
				nuci := nucs[i]
				var nuci_1 int
				if i-1 > -1 {
					nuci_1 = nucs[i-1]
				} else {
					nuci_1 = -1
				}

				// 2. M = P
				if i > 0 && j < seq_length-1 {
					newscore := score_M1(i, j, j, nuci_1, nuci, nucj, nucj1, seq_length) + state.score
					if (*beamstepM)[i] == nil {
						(*beamstepM)[i] = NewState(VALUE_MIN, MANNER_NONE)
					}
					update_if_better((*beamstepM)[i], newscore, MANNER_M_eq_P)
					nos_M++
				}

				// 3. M2 = M + P
				if !use_cube_pruning {
					k := i - 1
					if k > 0 && len(bestM[k]) != 0 {
						M1_score := score_M1(i, j, j, nuci_1, nuci, nucj, nucj1, seq_length) + state.score
						// candidate list
						// bestM2_iter := (*beamstepM2)[i]
						for newi, state := range bestM[k] {
							// eq. to first convert P to M1, then M2/M = M + M1
							newscore := M1_score + state.score
							if (*beamstepM2)[newi] == nil {
								(*beamstepM2)[newi] = NewState(VALUE_MIN, MANNER_NONE)
							}
							update_if_better3((*beamstepM2)[newi], newscore, MANNER_M2_eq_M_plus_P, k)
							//update_if_better(bestM[j][newi], newscore, MANNER_M_eq_M_plus_P, k);
							nos_M2++
							//++nos_M;
						}
					}
				}

				// 4. C = C + P
				{
					k := i - 1
					if k >= 0 {
						var prefix_C State = *bestC[k]
						if prefix_C.manner != MANNER_NONE {
							nuck := nuci_1
							nuck1 := nuci

							newscore := score_external_paired(k+1, j, nuck, nuck1,
								nucj, nucj1, seq_length) + prefix_C.score + state.score

							if beamstepC == nil {
								beamstepC = NewState(VALUE_MIN, MANNER_NONE)
							}
							update_if_better3(beamstepC, newscore, MANNER_C_eq_C_plus_P, k)
							nos_C++
						}
					} else {
						newscore := score_external_paired(0, j, -1, nucs[0],
							nucj, nucj1, seq_length) + state.score
						if beamstepC == nil {
							beamstepC = NewState(VALUE_MIN, MANNER_NONE)
						}
						update_if_better3(beamstepC, newscore, MANNER_C_eq_C_plus_P, -1)
						nos_C++
					}
				}
				//printf(" C = C + P at %d\n", j); fflush(stdout);

				// 1. generate new helix / single_branch
				// new state is of shape p..i..j..q
				if i > 0 && j < seq_length-1 {
					var precomputed float64 = score_junction_B(j, i, nucj, nucj1, nuci_1, nuci)
					for p := i - 1; p >= Max(i-SINGLE_MAX_LEN, 0); p-- {
						nucp := nucs[p]
						nucp1 := nucs[p+1]
						q := next_pair[nucp][j]

						for q != -1 && ((i-p)+(q-j)-2 <= SINGLE_MAX_LEN) {
							nucq := nucs[q]
							nucq_1 := nucs[q-1]

							if p == i-1 && q == j+1 {
								// helix
								var newscore float64 = score_helix(nucp, nucp1, nucq_1, nucq) + state.score
								if bestP[q][p] == nil {
									bestP[q][p] = NewState(VALUE_MIN, MANNER_NONE)
								}
								update_if_better(bestP[q][p], newscore, MANNER_HELIX)
								nos_P++
							} else {
								// single branch

								var newscore float64 = score_junction_B(p, q, nucp, nucp1, nucq_1, nucq) +
									precomputed +
									score_single_without_junctionB(p, q, i, j,
										nuci_1, nuci, nucj, nucj1) +
									state.score

								if bestP[q][p] == nil {
									bestP[q][p] = NewState(VALUE_MIN, MANNER_NONE)
								}
								update_if_better2(bestP[q][p], newscore, MANNER_SINGLE,
									rune(i-p), q-j)
								nos_P++
							}
							q = next_pair[nucp][q]
						}
					}
				}
				//printf(" helix / single at %d\n", j); fflush(stdout);
			}

			if use_cube_pruning {
				// 3. M2 = M + P with cube pruning
				var valid_Ps []int
				var M1_scores []float64

				for i, state := range *beamstepP {
					nuci := nucs[i]
					var nuci_1 int
					if i-1 > -1 {
						nuci_1 = nucs[i-1]
					} else {
						nuci_1 = -1
					}

					k := i - 1

					// group candidate Ps
					if k > 0 && len(bestM[k]) != 0 {
						if len(bestM[k]) != len(sorted_bestM[k]) {
							panic("bestM size != sorted_bestM size")
						}

						M1_score := score_M1(i, j, j, nuci_1, nuci, nucj, nucj1, seq_length) + state.score

						// bestM2_iter = beamstepM2[i]

						valid_Ps = append(valid_Ps, i)
						M1_scores = append(M1_scores, M1_score)

					}
				}

				// build max heap
				// heap is of form (heuristic score, (index of i in valid_Ps, index of M in bestM[i-1]))
				// vector<pair<value_type, pair<int, int>>> heap;
				// var heap []Pair
				h := &PairHeap{}
				heap.Init(h)
				for p := 0; p < len(valid_Ps); p++ {
					i := valid_Ps[p]
					k := i - 1
					heap.Push(h, Pair{M1_scores[p] + sorted_bestM[k][0].first.(float64), Pair{p, 0}})
					// heap = append(heap, Pair{M1_scores[p] + sorted_bestM[k][0].first.(float64), Pair{p, 0}})
					// HEAP STUFF
					// ±±±±±±±±push_heap(heap.begin(), heap.end())
				}

				// start cube pruning
				// stop after beam size M2 states being filled
				var filled int = 0
				// exit when filled >= beam and current score < prev score
				var prev_score float64 = VALUE_MIN
				var current_score float64 = VALUE_MIN
				for (filled < beam_size || current_score == prev_score) && len(*h) != 0 {

					// HEAP STUFF
					top := (*h)[0]
					// auto & top = heap.front()
					prev_score = current_score
					current_score = top.first.(float64)
					index_P := top.second.(Pair).first.(int)
					index_M := top.second.(Pair).second.(int)
					i := valid_Ps[top.second.(Pair).first.(int)]
					k := i - 1
					newi := sorted_bestM[k][index_M].second.(int)
					var newscore float64 = M1_scores[index_P] + bestM[k][newi].score
					// HEAP STUFF
					// pop the greatest element off the heap
					// pop_heap(heap.begin(), heap.end())
					// heap.pop_back()
					heap.Pop(h)

					if (*beamstepM2)[newi].manner == MANNER_NONE {
						filled++
						if (*beamstepM2)[newi] == nil {
							(*beamstepM2)[newi] = NewState(VALUE_MIN, MANNER_NONE)
						}
						update_if_better3((*beamstepM2)[newi], newscore, MANNER_M2_eq_M_plus_P, k)
						nos_M2++
					} else {
						if !((*beamstepM2)[newi].score > newscore-1e-8) {
							panic("beamstepM2[newi].score <= newscore - 1e-8")
						}
					}

					index_M++
					for index_M < len(sorted_bestM[k]) {
						// candidate_score is a heuristic score
						var candidate_score float64 = M1_scores[index_P] + sorted_bestM[k][index_M].first.(float64)
						var candidate_newi int = sorted_bestM[k][index_M].second.(int)

						var last State
						for _, v := range *beamstepM2 {
							last = *v
						}

						if *(*beamstepM2)[candidate_newi] == last {
							// HEAP STUFF
							heap.Push(h, Pair{candidate_score, Pair{index_P, index_M}})
							break
						} else {
							// based on the property of cube pruning, the new score must be worse
							// than the state already inserted
							// so we keep iterate through the candidate list to find the next
							// candidate
							index_M++
							if !((*beamstepM2)[candidate_newi].score >
								M1_scores[index_P]+bestM[k][candidate_newi].score-1e-8) {
								panic("beamstepM2[candidate_newi].score <= M1_scores[index_P] + bestM[k][candidate_newi].score - 1e-8")
							}
						}
					}
				}
			}
		}

		// beam of M2
		{
			if beam_size > 0 && len(*beamstepM2) > beam_size {
				BeamPrune(beamstepM2)
			}

			// for every state in M2[j]
			//   1. multi-loop  (by extending M2 on the left)
			//   2. M = M2
			for i, state := range *beamstepM2 {
				// 2. M = M2
				{
					if (*beamstepM)[i] == nil {
						(*beamstepM)[i] = NewState(VALUE_MIN, MANNER_NONE)
					}
					update_if_better((*beamstepM)[i], state.score, MANNER_M_eq_M2)
					nos_M++
				}

				// 1. multi-loop
				{
					for p := i - 1; p >= Max(i-SINGLE_MAX_LEN, 0); p-- {
						nucp := nucs[p]
						q := next_pair[nucp][j]

						if q != -1 && ((i - p - 1) <= SINGLE_MAX_LEN) {
							// the current shape is p..i M2 j ..q
							var newscore float64 = score_multi_unpaired(p+1, i-1) +
								score_multi_unpaired(j+1, q-1) + state.score

							if bestMulti[q][p] == nil {
								bestMulti[q][p] = NewState(VALUE_MIN, MANNER_NONE)
							}
							update_if_better2(bestMulti[q][p], newscore, MANNER_MULTI,
								rune(i-p),
								q-j)
							nos_Multi++
							//q = next_pair[nucp][q];
						}
					}
				}
			}
		}

		// beam of M
		{
			var threshold float64 = VALUE_MIN
			if beam_size > 0 && len(*beamstepM) > beam_size {
				threshold = BeamPrune(beamstepM)
			}

			sortM(threshold, beamstepM, sorted_bestM[j])

			// for every state in M[j]
			//   1. M = M + unpaired
			for i, state := range *beamstepM {
				if j < seq_length-1 {
					var newscore float64 = score_multi_unpaired(j+1, j+1) + state.score
					if bestM[j+1][i] == nil {
						bestM[j+1][i] = NewState(VALUE_MIN, MANNER_NONE)
					}
					update_if_better(bestM[j+1][i], newscore, MANNER_M_eq_M_plus_U)
					nos_M++
				}
			}
		}

		// beam of C
		{
			// C = C + U
			if j < seq_length-1 {
				var newscore float64 = score_external_unpaired(j+1, j+1) + beamstepC.score
				if bestC[j+1] == nil {
					bestC[j+1] = NewState(VALUE_MIN, MANNER_NONE)
				}
				update_if_better(bestC[j+1], newscore, MANNER_C_eq_C_plus_U)
				nos_C++
			}
		}
	} // end of for-loo j

	var viterbi *State = bestC[seq_length-1]

	// char result[seq_length + 1];
	result := get_parentheses(sequence)

	return result, viterbi.score
}

func ScoreExternalUnpaired(i, j int) float64 {
	return (float64(j) - float64(i) + 1) * external_unpaired
}

func BeamPrune(beamstep *map[int]*State) float64 {
	scores = make([]Pair, 0)
	for i, cand := range *beamstep {
		k := i - 1
		var newscore float64
		if (k >= 0) && (bestC[k].score == VALUE_MIN) {
			newscore = VALUE_MIN
		} else if k >= 0 {
			newscore = bestC[k].score + cand.score
		} else {
			newscore = cand.score
		}
		scores = append(scores, Pair{newscore, i})
	}

	if len(scores) <= beam_size {
		return VALUE_MIN
	}

	var threshold float64 = quickselect(0, len(scores)-1, len(scores)-beam_size)

	for _, pair := range scores {
		if pair.first.(float64) < threshold {
			delete(*beamstep, pair.second.(int))
		}
	}

	return threshold
}

func score_hairpin(i, j, nuci, nuci1, nucj_1, nucj int) float64 {
	return hairpin_length[Min(j-i-1, HAIRPIN_MAX_LEN)] +
		score_junction_B(i, j, nuci, nuci1, nucj_1, nucj)
}

func score_junction_B(i, j, nuci, nuci1, nucj_1, nucj int) float64 {
	return helix_closing_score(nuci, nucj) + terminal_mismatch_score(nuci, nuci1, nucj_1, nucj)
}

func helix_closing_score(nuci, nucj int) float64 {
	return helix_closing[nuci*NOTON+nucj]
}

func terminal_mismatch_score(nuci, nuci1, nucj_1, nucj int) float64 {
	return terminal_mismatch[nuci*NOTONT+nucj*NOTOND+nuci1*NOTON+nucj_1]
}

// in-place quick-select
func quickselect(lower int, upper int, k int) float64 {

	if lower == upper {
		return scores[lower].first.(float64)
	}
	var split int = quickselect_partition(lower, upper)
	var length int = split - lower + 1

	if length == k {
		return scores[split].first.(float64)
	} else if k < length {
		return quickselect(lower, split-1, k)
	} else {
		return quickselect(split+1, upper, k-length)
	}
}

func quickselect_partition(lower int, upper int) int {
	var pivot float64 = scores[upper].first.(float64)
	for lower < upper {
		for scores[lower].first.(float64) < pivot {
			lower++
		}
		for scores[upper].first.(float64) > pivot {
			upper--
		}
		if scores[lower].first == scores[upper].first {
			lower++
		} else if lower < upper {
			scores[lower], scores[upper] = scores[upper], scores[lower]
		}
	}
	return upper
}

func score_M1(i, j, k, nuci_1, nuci, nuck, nuck1, len int) float64 {
	return score_junction_A(k, i, nuck, nuck1, nuci_1, nuci, len) +
		score_multi_unpaired(k+1, j) + base_pair_score(nuci, nuck) + multi_paired
}

func score_junction_A(i, j, nuci, nuci1, nucj_1, nucj, len int) float64 {
	var result float64 = helix_closing_score(nuci, nucj)
	if i < len-1 {
		result += dangle_left_score(nuci, nuci1, nucj)
	}
	if j > 0 {
		result += dangle_right_score(nuci, nucj_1, nucj)
	}
	return result
}

func score_multi_unpaired(i, j int) float64 {
	return (float64(j) - float64(i) + 1) * multi_unpaired
}

func base_pair_score(nuci, nucj int) float64 {
	return base_pair[nucj*NOTON+nuci]
}

func dangle_left_score(nuci, nuci1, nucj int) float64 {
	return dangle_left[nuci*NOTOND+nucj*NOTON+nuci1]
}

// parameters: nucs[i], nucs[j-1], nucs[j]
func dangle_right_score(nuci, nucj_1, nucj int) float64 {
	return dangle_right[nuci*NOTOND+nucj*NOTON+nucj_1]
}

func score_external_paired(i, j, nuci_1, nuci, nucj, nucj1, len int) float64 {
	return score_junction_A(j, i, nucj, nucj1, nuci_1, nuci, len) +
		external_paired + base_pair_score(nuci, nucj)
}

func score_helix(nuci, nuci1, nucj_1, nucj int) float64 {
	return helix_stacking_score(nuci, nuci1, nucj_1, nucj) + base_pair_score(nuci1, nucj_1)
}

// parameters: nucs[i], nucs[i+1], nucs[j-1], nucs[j]
func helix_stacking_score(nuci, nuci1, nucj_1, nucj int) float64 {
	return helix_stacking[nuci*NOTONT+nucj*NOTOND+nuci1*NOTON+nucj_1]
}

func sortM(threshold float64, beamstep *map[int]*State, sorted_stepM []Pair) {
	sorted_stepM = make([]Pair, 0)
	if threshold == VALUE_MIN {
		// no beam pruning before, so scores vector not usable
		for i, cand := range *beamstep {
			k := i - 1
			var newscore float64
			if k >= 0 {
				newscore = bestC[k].score + cand.score
			} else {
				newscore = cand.score
			}
			sorted_stepM = append(sorted_stepM, Pair{newscore, i})
		}
	} else {
		for _, pair := range scores {
			if pair.first.(float64) >= threshold {
				sorted_stepM = append(sorted_stepM, pair)
			}
		}
	}

	// sort.Sort()
	sort.Slice(sorted_stepM, func(i, j int) bool {
		return sorted_stepM[i].first.(float64) > sorted_stepM[j].first.(float64)
	})
}

func score_external_unpaired(i, j int) float64 {
	return (float64(j) - float64(i) + 1) * external_unpaired
}

type Tuple struct {
	first, second, third interface{}
}

func get_parentheses(seq string) string {
	// is_verbose := true

	seq_length := len(seq)
	result := make([]rune, seq_length)
	for i := 0; i < seq_length; i++ {
		result[i] = '.'
	}

	var stk []Tuple
	// stack<tuple<int, int, State>> stk;
	stk = append(stk, Tuple{0, seq_length - 1, bestC[seq_length-1]})

	// verbose stuff
	// var multi_todo []Pair
	// var mbp map[int]int // multi bp

	// var total_energy float64 = .0
	// var external_energy float64 = .0

	for len(stk) != 0 {
		last_elem_idx := len(stk) - 1
		top := stk[last_elem_idx]
		i, j, state := top.first.(int), top.second.(int), top.third.(*State)

		// pop off stack
		// stk[last_elem_idx] = nil
		stk = stk[:last_elem_idx]

		var k, p, q int

		switch state.manner {
		case MANNER_H:
			// this state should not be traced
		case MANNER_HAIRPIN:
			{
				result[i] = '('
				result[j] = ')'
				// if (is_verbose) {
				// 	var tetra_hex_tri int = -1
				// 	if (j - i - 1 == 4) {
				// 		// 6:tetra
				// 		tetra_hex_tri = if_tetraloops[i]
				// 	} else if (j - i - 1 == 6) {
				// 		// 8:hexa
				// 		tetra_hex_tri = if_hexaloops[i]
				// 	} else if (j - i - 1 == 3) {
				// 		// 5:tri
				// 		tetra_hex_tri = if_triloops[i]
				// 		var nuci, nucj int = nucs[i], nucs[j]

				// 		var nuci1 int
				// 		if (i + 1) < seq_length {
				// 			nuci1 = nucs[i + 1]
				// 		} else {
				// 			nuci1 = -1
				// 		}

				// 		var nucj_1 int
				// 		if (j - 1) > -1 {
				// 			nucj_1 = nucs[j - 1]
				// 		} else {
				// 			nucj_1 = -1
				// 		}

				// 		var newscore float64 = -v_score_hairpin(i, j, nuci, nuci1, nucj_1, nucj, tetra_hex_tri);
				// 		fmt.Printf("Hairpin loop ( %d, %d) %c%c : %.2f\n", i + 1, j + 1, seq[i], seq[j], newscore / -100.0);
				// 		total_energy += newscore;
				// 	}
				// }
			}
		case MANNER_SINGLE:
			{
				result[i] = '('
				result[j] = ')'
				p = i + int(state.trace.paddings.l1)
				q = j - state.trace.paddings.l2
				stk = append(stk, Tuple{p, q, bestP[q][p]})
				// if (is_verbose)
				// {
				// 		int nuci = nucs[i], nuci1 = nucs[i + 1], nucj_1 = nucs[j - 1], nucj = nucs[j];
				// 		int nucp_1 = nucs[p - 1], nucp = nucs[p], nucq = nucs[q], nucq1 = nucs[q + 1];

				// 		value_type newscore = -v_score_single(i, j, p, q, nuci, nuci1, nucj_1, nucj,
				// 																					nucp_1, nucp, nucq, nucq1);
				// 		printf("Interior loop ( %d, %d) %c%c; ( %d, %d) %c%c : %.2f\n", i + 1, j + 1, seq[i], seq[j], p + 1, q + 1, seq[p], seq[q], newscore / -100.0);
				// 		total_energy += newscore;
				// }
			}
		case MANNER_HELIX:
			{
				result[i] = '('
				result[j] = ')'
				stk = append(stk, Tuple{i + 1, j - 1, bestP[j-1][i+1]})
				// if (is_verbose)
				// {
				// 		p = i + 1;
				// 		q = j - 1;
				// 		int nuci = nucs[i], nuci1 = nucs[i + 1], nucj_1 = nucs[j - 1], nucj = nucs[j];
				// 		int nucp_1 = nucs[p - 1], nucp = nucs[p], nucq = nucs[q], nucq1 = nucs[q + 1];

				// 		value_type newscore = -v_score_single(i, j, p, q, nuci, nuci1, nucj_1, nucj,
				// 																					nucp_1, nucp, nucq, nucq1);
				// 		printf("Interior loop ( %d, %d) %c%c; ( %d, %d) %c%c : %.2f\n", i + 1, j + 1, seq[i], seq[j], p + 1, q + 1, seq[p], seq[q], newscore / -100.0);
				// 		total_energy += newscore;
				// }
			}
		case MANNER_MULTI:
			p = i + int(state.trace.paddings.l1)
			q = j - state.trace.paddings.l2
			stk = append(stk, Tuple{p, q, bestM2[q][p]})
			// stk.push(make_tuple(p, q, bestM2[q][p]));
		case MANNER_MULTI_eq_MULTI_plus_U:
			p = i + int(state.trace.paddings.l1)
			q = j - state.trace.paddings.l2
			stk = append(stk, Tuple{p, q, bestM2[q][p]})
		case MANNER_P_eq_MULTI:
			result[i] = '('
			result[j] = ')'
			stk = append(stk, Tuple{i, j, bestMulti[j][i]})
			// if (is_verbose) {
			// 		multi_todo.push_back(make_pair(i, j));
			// }
		case MANNER_M2_eq_M_plus_P:
			k = state.trace.split
			stk = append(stk, Tuple{i, k, bestM[k][i]})
			stk = append(stk, Tuple{k + 1, j, bestP[j][k+1]})
			// if (is_verbose) {
			// 	mbp[k + 1] = j
			// }
			// break
		case MANNER_M_eq_M2:
			stk = append(stk, Tuple{i, j, bestM2[j][i]})
			// stk.push(make_tuple(i, j, bestM2[j][i]));
			// break
		case MANNER_M_eq_M_plus_U:
			stk = append(stk, Tuple{i, j - 1, bestM[j-1][i]})
			// break
		case MANNER_M_eq_P:
			stk = append(stk, Tuple{i, j, bestP[j][i]})
			// if (is_verbose) {
			// 	mbp[i] = j;
			// }
		case MANNER_C_eq_C_plus_U:
			k = j - 1
			if k != -1 {
				stk = append(stk, Tuple{0, k, bestC[k]})
			}
		// if (is_verbose)
		// 		external_energy += -v_score_external_unpaired(0, 0); // zero at this moment
		case MANNER_C_eq_C_plus_P:
			{
				k = state.trace.split
				if k != -1 {
					stk = append(stk, Tuple{0, k, bestC[k]})
					stk = append(stk, Tuple{k + 1, j, bestP[j][k+1]})
				} else {
					stk = append(stk, Tuple{i, j, bestP[j][i]})
				}
				// if (is_verbose)
				// {
				// 		int nuck = k > -1 ? nucs[k] : -1;
				// 		int nuck1 = nucs[k + 1], nucj = nucs[j];
				// 		int nucj1 = (j + 1) < seq_length ? nucs[j + 1] : -1;
				// 		external_energy += -v_score_external_paired(k + 1, j, nuck, nuck1,
				// 																								nucj, nucj1, seq_length);
				// }
			}
		default: // MANNER_NONE or other cases
			fmt.Printf("wrong manner at %d, %d: manner %d\n", i, j, state.manner)
			// fflush(stdout);
			// assert(false);
			panic("wrong manner")
		}
	}

	// 	if (is_verbose)
	// 	{
	// 			for (auto item : multi_todo)
	// 			{
	// 					int i = item.first;
	// 					int j = item.second;
	// 					int nuci = nucs[i], nuci1 = nucs[i + 1], nucj_1 = nucs[j - 1], nucj = nucs[j];
	// 					value_type multi_energy = -v_score_multi(i, j, nuci, nuci1, nucj_1, nucj, seq_length);
	// 					int num_unpaired = 0;
	// 					for (int k = i + 1; k < j; ++k)
	// 					{
	// 							if (result[k] == '.')
	// 									num_unpaired += 1;
	// 							else if (result[k] == '(')
	// 							{
	// 									int p = k, q = mbp[k];
	// 									int nucp_1 = nucs[p - 1], nucp = nucs[p], nucq = nucs[q], nucq1 = nucs[q + 1];

	// 									multi_energy += -v_score_M1(p, q, q, nucp_1, nucp, nucq, nucq1, seq_length);
	// 									k = q;
	// 							}
	// 					}
	// 					multi_energy += -v_score_multi_unpaired(1, num_unpaired);

	// 					printf("Multi loop ( %d, %d) %c%c : %.2f\n", i + 1, j + 1, seq[i], seq[j], multi_energy / -100.0);
	// 					total_energy += multi_energy;
	// 			}

	// 			printf("External loop : %.2f\n", external_energy / -100.0);
	// 			total_energy += external_energy;

	// #ifndef lv
	// 			printf("Energy(kcal/mol): %.2f\n", total_energy / -100.0);
	// #endif
	// 	}

	return string(result)
}

// func v_score_hairpin(i, j, nuci, nuci1, nucj_1, nucj, tetra_hex_tri_index int) int {
// 	size, type, si1, sj1 := j-i-1, NUM_TO_PAIR(nuci, nucj), NUM_TO_NUC(nuci1), NUM_TO_NUC(nucj_1)
// 	var energy int
// 	if(size <= 30) {
// 		energy = hairpin37[size]
// 	} else {
// 		energy = hairpin37[30] + (int)(lxc37*log((size)/30.))
// 	}

// 	if(size < 3) return energy; /* should only be the case when folding alignments */
// #ifdef SPECIAL_HP
// 	// if(special_hp){
// 			if (size == 4 && tetra_hex_tri_index > -1)
// 					return Tetraloop37[tetra_hex_tri_index];
// 			else if (size == 6 && tetra_hex_tri_index > -1)
// 					return Hexaloop37[tetra_hex_tri_index];
// 			else if (size == 3) {
// 					if (tetra_hex_tri_index > -1)
// 							return Triloop37[tetra_hex_tri_index];
// 					return (energy + (type>2 ? TerminalAU37 : 0));
// 			}
// 	// }
// #endif

// 	energy += mismatchH37[type][si1][sj1];

// 	return energy;
// }

func NUM_TO_NUC(x int) int {
	switch x {
	case -1:
		return -1
	case 4:
		return 0
	default:
		return x + 1
	}
}

func NUM_TO_PAIR(x, y int) int {
	switch x {
	case 0:
		if y == 3 {
			return 5
		} else {
			return 0
		}
	case 1:
		if y == 2 {
			return 1
		} else {
			return 0
		}
	case 2:
		if y == 1 {
			return 2
		} else {
			if y == 3 {
				return 3
			} else {
				return 0
			}
		}
	case 3:
		if y == 2 {
			return 4
		} else if y == 0 {
			return 6
		} else {
			return 0
		}
	default:
		return 0
	}
}

func score_multi(i, j, nuci, nuci1, nucj_1, nucj, len int) float64 {
	return score_junction_A(i, j, nuci, nuci1, nucj_1, nucj, len) +
		multi_paired + multi_base
}

func score_single_without_junctionB(i, j, p, q, nucp_1, nucp, nucq, nucq1 int) float64 {
	var l1, l2 int = p - i - 1, j - q - 1
	return cache_single[l1][l2] + base_pair_score(nucp, nucq) +
		score_single_nuc(i, j, p, q, nucp_1, nucq1)
}

func score_single_nuc(i, j, p, q, nucp_1, nucq1 int) float64 {
	var l1, l2 int = p - i - 1, j - q - 1
	if l1 == 0 && l2 == 1 {
		return bulge_nuc_score(nucq1)
	} else if l1 == 1 && l2 == 0 {
		return bulge_nuc_score(nucp_1)
	} else if l1 == 1 && l2 == 1 {
		return internal_nuc_score(nucp_1, nucq1)
	}
	return 0
}

func bulge_nuc_score(nuci int) float64 {
	return bulge_0x1_nucleotides[nuci]
}

// parameters: nucs[i], nucs[j]
func internal_nuc_score(nuci, nucj int) float64 {
	return internal_1x1_nucleotides[nuci*NOTON+nucj]
}
