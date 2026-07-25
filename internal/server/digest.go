package server

import "smellyfeet/internal/apiclient"

// articleGroup is one story: the newest article covering it, plus every other
// article the backend's clustering job put in the same cluster.
type articleGroup struct {
	Lead    apiclient.Article
	Members []apiclient.Article
}

// groupByCluster collapses a digest slice into one group per story cluster,
// so a story covered by four feeds renders once instead of four times.
//
// Incoming order (cross_feed_count DESC, publish_date DESC, set by the API) is
// preserved: groups appear in order of first appearance, and the first article
// seen for a cluster leads it. Every member of a cluster shares the same
// cross_feed_count, so within a cluster the incoming order is purely
// publish_date DESC -- which makes the first-seen article the newest one.
//
// A nil StoryClusterID means the clustering job hasn't reached that article
// yet -- "unclassified", not "same story" -- so each one becomes its own
// singleton group rather than being lumped in with the other nils. This is
// also the whole-page behaviour against a backend that predates the field,
// which degrades to the flat list this page used to render.
func groupByCluster(articles []apiclient.Article) []articleGroup {
	groups := []articleGroup{}
	indexByCluster := map[int64]int{}
	for _, a := range articles {
		if a.StoryClusterID == nil {
			groups = append(groups, articleGroup{Lead: a})
			continue
		}
		if i, ok := indexByCluster[*a.StoryClusterID]; ok {
			groups[i].Members = append(groups[i].Members, a)
			continue
		}
		indexByCluster[*a.StoryClusterID] = len(groups)
		groups = append(groups, articleGroup{Lead: a})
	}
	return groups
}
