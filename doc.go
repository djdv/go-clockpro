// Package clockpro implements a [Cache] using the CLOCK‑Pro+ replacement algorithm.
//
// CLOCK‑Pro+ is an adaptive, scan‑resistant policy that balances recency
// and frequency to improve hit rates over traditional CLOCK and LRU
// by utilizing adaptive hot/cold targets and a bounded metadata "test" set.
//
// The following is a summary (intended for maintainers)
// of several papers, which are recursively cited by the CLOCK-Pro+ paper.
// Most of the clock behaviour+documentation is derived from the [2005 USENIX CLOCK-Pro paper]
// and then adapted via the [CLOCK-PRO+ paper].
//
// Glossary and invariants:
//
//   - Page contains metadata along with a potentially valid/"resident" value.
//
//   - LIR: Low Inter-reference Recency (hot) page.
//
//     Resident and protected from eviction.
//
//   - HIR: High Inter-reference Recency (cold) page.
//
//     May be resident or nonresident.
//
//   - Test page
//
//     Nonresident HIR page retained only as metadata to guide adaptation.
//
//   - Stacked
//
//     Whether the page currently exists in the stack `S` (metadata ring).
//
//   - Resident
//
//     Whether the page's value is in cache (as opposed to metadata-only).
//
//   - Referenced
//
//     Set on access; cleared when a hand processes the page.
//
//   - Demoted
//
//     Set when an LIR is demoted to HIR until it is later referenced/processed.
//
// Operations:
//
//   - Eviction
//
//     When a resident HIR (cold) page is removed its value is discarded,
//     but its metadata may remain as a nonresident “test page” to inform future hot/cold target adaption/adjustment.
//
//   - Demotion
//
//     When an LIR (hot) page is converted into an HIR (cold) page and moves to the LRU,
//     because the hot set exceeds its target size.
//
// Hands:
//
//   - hot
//
//     Scans until it finds an LIR with Referenced == false.
//     Clears references and updates LRU; removes nonresident HIR page it encounters.
//
//   - cold
//
//     Scans until it finds an unreferenced resident HIR to evict.
//     On referenced HIRs: clears reference and either promotes (if stacked) or restacks.
//
//   - test
//
//     Points to a nonresident, non-LIR HIR (a "test page") when testCount > 0.
//
//   - lru
//
//     The tail of the ring/"recency stack"; new/updated pages are moved to lru.
//
// Counts and targets:
//
//   - hotCount + coldCount == resident capacity.
//
//     The abstract "length" of the cache / usable page count.
//
//   - testCount == number of metadata-only pages.
//
//     Some pages are kept around to make adaption decisions.
//
//   - coldTarget ∈ [1, capacit/2], hotTarget = capacity - coldTarget.
//
//     This is some point within the range that the hot and cold page sizes can be adapted to.
//
//   - Metadata is bounded to: hotCount + coldCount + testCount ≤ 2 * size.
//
//     "So totally there are at most `2m` metadata pages for keeping track of page access history in the list."
//
//     This bound ensures adaptation history without unbounded growth.
//
// [2005 USENIX CLOCK-Pro paper]: https://www.usenix.org/conference/2005-usenix-annual-technical-conference/clock-pro-effective-improvement-clock-replacement
// [CLOCK-PRO+ paper]: https://dl.acm.org/doi/10.1145/3319647.3325838
package clockpro
