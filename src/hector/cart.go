package hector

import (
	"strconv"
	"sort"
	"container/list"
	"fmt"
)

type CART struct {
	tree Tree
	params CARTParams
	continuous_features bool
}

func (rdt *CART) GoLeft(sample *MapBasedSample, feature_split Feature) bool {
	value, ok := sample.Features[feature_split.Id]
	if ok && value >= feature_split.Value {
		return true
	} else {
		return false
	}
}

func (dt *CART) GetElementFromQueue(queue *list.List, n int) []*TreeNode {
	ret := []*TreeNode{}
	for i := 0; i < n; i++ {
		node := queue.Front()
		if node == nil{
			break
		}
		ret = append(ret, (node.Value.(*TreeNode)))
		queue.Remove(node)
	}
	return ret
}

func (dt *CART) FindBestSplitOfContinusousFeature(samples []*MapBasedSample, node *TreeNode, select_features map[int64]bool){
	feature_weight_labels := make(map[int64]*FeatureLabelDistribution)
	positive := 0
	total := 0
	for _, k := range node.samples{
		total += 1
		positive += int(samples[k].Label)
		for fid, fvalue := range samples[k].Features{
			if select_features != nil {
				_, ok := select_features[fid]
				if !ok {
					continue
				}
			}
			_, ok := feature_weight_labels[fid]
			if !ok {
				feature_weight_labels[fid] = NewFeatureLabelDistribution()
			}	
			feature_weight_labels[fid].AddWeightLabel(fvalue, samples[k].Label)
		}
	}
	
	min_gini := 1.0
	node.feature_split = Feature{Id:-1, Value: 0}
	for fid, distribution := range feature_weight_labels{
		sort.Sort(distribution)
		split, gini := distribution.BestSplitByGini(total, positive)
		if min_gini > gini {
			min_gini = gini
			node.feature_split.Id = fid
			node.feature_split.Value = split
		}
	}
	if min_gini > dt.params.GiniThreshold {
		node.feature_split.Id = -1
		node.feature_split.Value = 0.0
	}
}

func (dt *CART) FindBestSplitOfBinaryFeature(samples []*MapBasedSample, node *TreeNode, select_features map[int64]bool){
	feature_positive := NewVector()
	feature_total := NewVector()
	positive := 0.0
	total := 0.0
	for _, k := range node.samples{
		total += 1.0
		positive += samples[k].Label
		for fid, _ := range samples[k].Features{
			if select_features != nil {
				_, ok := select_features[fid]
				if !ok {
					continue
				}
			}
			feature_positive.AddValue(fid, samples[k].Label)
			feature_total.AddValue(fid, 1.0)
		}
	}
	
	min_gini := 1.0
	node.feature_split = Feature{Id:-1, Value: 0}
	for fid, ftotal := range feature_total.data{
		gini := Gini(positive - feature_positive.GetValue(fid), total - ftotal, feature_positive.GetValue(fid), ftotal)
		if min_gini > gini {
			min_gini = gini
			node.feature_split.Id = fid
			node.feature_split.Value = 1.0
		}
	}
	if min_gini > dt.params.GiniThreshold {
		node.feature_split.Id = -1
		node.feature_split.Value = 0.0
	}
}


func (dt *CART) AppendNodeToTree(samples []*MapBasedSample, node *TreeNode, queue *list.List, tree *Tree, select_features map[int64]bool) {
	if node.depth >= dt.params.MaxDepth {
		return
	}
	
	if dt.continuous_features {
		dt.FindBestSplitOfContinusousFeature(samples, node, select_features)
	} else {
		dt.FindBestSplitOfBinaryFeature(samples, node, select_features)
	}
	if node.feature_split.Id < 0{
		return
	}
	left_node := TreeNode{depth: node.depth + 1, left: -1, right: -1, prediction: -1, sample_count: 0, samples: []int{}}
	right_node := TreeNode{depth: node.depth + 1, left: -1, right: -1, prediction: -1, sample_count: 0, samples: []int{}}

	left_positive := 0.0
	left_total := 0.0
	right_positive := 0.0
	right_total := 0.0
	for _, k := range node.samples {
		if dt.GoLeft(samples[k], node.feature_split) {
			left_node.samples = append(left_node.samples, k)
			left_positive += samples[k].LabelDoubleValue()
			left_total += 1.0
		} else {
			right_node.samples = append(right_node.samples, k)
			right_positive += samples[k].LabelDoubleValue()
			right_total += 1.0
		}
	}
	node.samples = nil
	
	if len(left_node.samples) > dt.params.MinLeafSize {
		left_node.sample_count = len(left_node.samples)
		left_node.prediction = left_positive / left_total
		queue.PushBack(&left_node)
		node.left = len(tree.nodes)
		tree.AddTreeNode(&left_node)
	}

	if len(right_node.samples) > dt.params.MinLeafSize {
		right_node.sample_count = len(right_node.samples)
		right_node.prediction = right_positive / right_total
		queue.PushBack(&right_node)
		node.right = len(tree.nodes)
		tree.AddTreeNode(&right_node)
	}
}

func (dt *CART) SingleTreeBuild(samples []*MapBasedSample, select_features map[int64]bool) Tree {
	tree := Tree{}
	queue := list.New()
	root := TreeNode{depth: 0, left: -1, right: -1, prediction: -1, samples: []int{}}
	total := 0.0
	positive := 0.0
	for i, sample := range samples {
		root.AddSample(i)
		total += 1.0
		positive += sample.LabelDoubleValue()
	}
	root.sample_count = len(root.samples)
	root.prediction = positive / total

	queue.PushBack(&root)
	tree.AddTreeNode(&root)
	for {
		nodes := dt.GetElementFromQueue(queue, 10)
		if len(nodes) == 0 {
			break
		}
		
		for _, node := range nodes {
			dt.AppendNodeToTree(samples, node, queue, &tree, select_features)
		}
	}
	return tree
}

func (dt *CART) PredictBySingleTree(tree *Tree, sample *MapBasedSample) (*TreeNode, string) {
	path := ""
	node := tree.GetNode(0)
	path += node.ToString()
	for {
		if dt.GoLeft(sample, node.feature_split) {
			if node.left >= 0 && node.left < tree.Size() {
				node = tree.GetNode(node.left)
				path += "-" + node.ToString()
			} else {
				break
			}
		} else {
			if node.right >= 0 && node.right < tree.Size() {
				node = tree.GetNode(node.right)
				path += "+" + node.ToString()
			} else {
				break
			}
		}
	}
	return node, path
}

func (dt *CART) Train(dataset * DataSet) {
	samples := []*MapBasedSample{}
	feature_weights := make(map[int64]float64)
	for _,sample := range dataset.Samples{
		if !dt.continuous_features {
			for _, f := range sample.Features {
				_, ok := feature_weights[f.Id]
				if !ok {
					feature_weights[f.Id] = f.Value
				}
				if feature_weights[f.Id] != f.Value {
					dt.continuous_features = true
				}
			}
		}
		msample := sample.ToMapBasedSample()
		samples = append(samples, msample)
	}
	if dt.continuous_features {
		fmt.Println("Continuous DataSet")
	} else {
		fmt.Println("Binary DataSet")
	}
	dt.tree = dt.SingleTreeBuild(samples, nil)
}

func (dt *CART) Predict(sample * Sample) float64 {
	msample := sample.ToMapBasedSample()
	node,_ := dt.PredictBySingleTree(&dt.tree, msample)
	return node.prediction
}


type CARTParams struct {
	MaxDepth   int
	MinLeafSize int
	GiniThreshold float64
}

func (dt *CART) Init(params map[string]string) {
	dt.tree = Tree{}
	dt.continuous_features = false
	min_leaf_size, _ := strconv.ParseInt(params["min-leaf-size"], 10, 32)
	max_depth, _ := strconv.ParseInt(params["max-depth"], 10, 32)
	
	dt.params.MinLeafSize = int(min_leaf_size)
	dt.params.MaxDepth = int(max_depth)
	dt.params.GiniThreshold, _ = strconv.ParseFloat(params["gini"], 64)
}

