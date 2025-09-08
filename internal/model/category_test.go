package model

import (
	"testing"
)

func TestCategory_GetParentCode(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected string
	}{
		{
			name:     "大类编码",
			code:     "1",
			expected: "",
		},
		{
			name:     "中类编码",
			code:     "1-01",
			expected: "1",
		},
		{
			name:     "小类编码",
			code:     "1-01-01",
			expected: "1-01",
		},
		{
			name:     "细类编码",
			code:     "1-01-01-01",
			expected: "1-01-01",
		},
		{
			name:     "空编码",
			code:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Category{Code: tt.code}
			result := c.GetParentCode()
			if result != tt.expected {
				t.Errorf("GetParentCode() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCategory_GetLevel(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected int
	}{
		{
			name:     "大类编码",
			code:     "1",
			expected: 0,
		},
		{
			name:     "中类编码",
			code:     "1-01",
			expected: 1,
		},
		{
			name:     "小类编码",
			code:     "1-01-01",
			expected: 2,
		},
		{
			name:     "细类编码",
			code:     "1-01-01-01",
			expected: 3,
		},
		{
			name:     "空编码",
			code:     "",
			expected: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Category{Code: tt.code}
			result := c.GetLevel()
			if result != tt.expected {
				t.Errorf("GetLevel() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestCategory_AddChild(t *testing.T) {
	parent := &Category{
		Code: "1",
		Name: "大类",
	}

	child1 := &Category{
		Code: "1-01",
		Name: "中类1",
	}

	child2 := &Category{
		Code: "1-02",
		Name: "中类2",
	}

	// 添加第一个子节点
	parent.AddChild(child1)
	if len(parent.Children) != 1 {
		t.Errorf("Expected 1 child, got %d", len(parent.Children))
	}
	if parent.Children[0].Code != "1-01" {
		t.Errorf("Expected child code '1-01', got '%s'", parent.Children[0].Code)
	}

	// 添加第二个子节点
	parent.AddChild(child2)
	if len(parent.Children) != 2 {
		t.Errorf("Expected 2 children, got %d", len(parent.Children))
	}
}

func TestCategory_GetChildrenCount(t *testing.T) {
	parent := &Category{Code: "1"}

	// 初始没有子节点
	if count := parent.GetChildrenCount(); count != 0 {
		t.Errorf("Expected 0 children, got %d", count)
	}

	// 添加子节点
	parent.AddChild(&Category{Code: "1-01"})
	parent.AddChild(&Category{Code: "1-02"})

	if count := parent.GetChildrenCount(); count != 2 {
		t.Errorf("Expected 2 children, got %d", count)
	}
}

func TestCategory_GetTotalDescendantsCount(t *testing.T) {
	// 构建测试层级结构
	root := &Category{Code: "1"}
	child1 := &Category{Code: "1-01"}
	child2 := &Category{Code: "1-02"}
	grandchild1 := &Category{Code: "1-01-01"}
	grandchild2 := &Category{Code: "1-01-02"}

	child1.AddChild(grandchild1)
	child1.AddChild(grandchild2)
	root.AddChild(child1)
	root.AddChild(child2)

	// root有2个直接子节点，2个孙节点，总共4个后代
	expected := 4
	result := root.GetTotalDescendantsCount()
	if result != expected {
		t.Errorf("Expected %d descendants, got %d", expected, result)
	}

	// child1有2个直接子节点
	expected = 2
	result = child1.GetTotalDescendantsCount()
	if result != expected {
		t.Errorf("Expected %d descendants, got %d", expected, result)
	}

	// 叶子节点没有后代
	expected = 0
	result = grandchild1.GetTotalDescendantsCount()
	if result != expected {
		t.Errorf("Expected %d descendants, got %d", expected, result)
	}
}

func TestCategory_FindChild(t *testing.T) {
	parent := &Category{Code: "1"}
	child1 := &Category{Code: "1-01", Name: "Child1"}
	child2 := &Category{Code: "1-02", Name: "Child2"}

	parent.AddChild(child1)
	parent.AddChild(child2)

	// 查找存在的子节点
	found := parent.FindChild("1-01")
	if found == nil {
		t.Error("Expected to find child with code '1-01'")
	} else if found.Name != "Child1" {
		t.Errorf("Expected child name 'Child1', got '%s'", found.Name)
	}

	// 查找不存在的子节点
	notFound := parent.FindChild("1-03")
	if notFound != nil {
		t.Error("Expected not to find child with code '1-03'")
	}

	// 在没有子节点的节点中查找
	leaf := &Category{Code: "2"}
	result := leaf.FindChild("2-01")
	if result != nil {
		t.Error("Expected not to find any child in leaf node")
	}
}

func TestCategory_FindDescendant(t *testing.T) {
	// 构建测试层级结构
	root := &Category{Code: "1"}
	child1 := &Category{Code: "1-01"}
	child2 := &Category{Code: "1-02"}
	grandchild1 := &Category{Code: "1-01-01", Name: "Grandchild1"}
	grandchild2 := &Category{Code: "1-01-02", Name: "Grandchild2"}

	child1.AddChild(grandchild1)
	child1.AddChild(grandchild2)
	root.AddChild(child1)
	root.AddChild(child2)

	// 查找自己
	found := root.FindDescendant("1")
	if found == nil || found.Code != "1" {
		t.Error("Expected to find self")
	}

	// 查找直接子节点
	found = root.FindDescendant("1-01")
	if found == nil || found.Code != "1-01" {
		t.Error("Expected to find direct child")
	}

	// 查找孙节点
	found = root.FindDescendant("1-01-01")
	if found == nil || found.Name != "Grandchild1" {
		t.Error("Expected to find grandchild")
	}

	// 查找不存在的节点
	notFound := root.FindDescendant("2-01")
	if notFound != nil {
		t.Error("Expected not to find non-existent descendant")
	}
}

func TestCategory_ToFlat(t *testing.T) {
	// 构建测试层级结构
	root := &Category{Code: "1", Name: "Root"}
	child1 := &Category{Code: "1-01", Name: "Child1"}
	child2 := &Category{Code: "1-02", Name: "Child2"}
	grandchild1 := &Category{Code: "1-01-01", Name: "Grandchild1"}

	child1.AddChild(grandchild1)
	root.AddChild(child1)
	root.AddChild(child2)

	// 转换为扁平列表
	flat := root.ToFlat()

	// 验证数量：root + child1 + child2 + grandchild1 = 4
	expected := 4
	if len(flat) != expected {
		t.Errorf("Expected %d items in flat list, got %d", expected, len(flat))
	}

	// 验证根节点是第一个
	if flat[0].Code != "1" || flat[0].Name != "Root" {
		t.Error("Expected root node to be first in flat list")
	}

	// 验证所有节点都在列表中
	codes := make(map[string]bool)
	for _, category := range flat {
		codes[category.Code] = true
	}

	expectedCodes := []string{"1", "1-01", "1-02", "1-01-01"}
	for _, code := range expectedCodes {
		if !codes[code] {
			t.Errorf("Expected code '%s' to be in flat list", code)
		}
	}
}

func TestParsedInfo_GetLevelName(t *testing.T) {
	tests := []struct {
		level    int
		expected string
	}{
		{0, LevelMajor},
		{1, LevelMiddle},
		{2, LevelSmall},
		{3, LevelDetail},
		{4, "未知层级"},
		{-1, "未知层级"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			info := &ParsedInfo{Level: tt.level}
			result := info.GetLevelName()
			if result != tt.expected {
				t.Errorf("GetLevelName() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestParsedInfo_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		info     *ParsedInfo
		expected bool
	}{
		{
			name: "有效数据",
			info: &ParsedInfo{
				Code:  "1-01-01",
				Name:  "测试分类",
				Level: 2,
			},
			expected: true,
		},
		{
			name: "缺少编码",
			info: &ParsedInfo{
				Code:  "",
				Name:  "测试分类",
				Level: 2,
			},
			expected: false,
		},
		{
			name: "缺少名称",
			info: &ParsedInfo{
				Code:  "1-01-01",
				Name:  "",
				Level: 2,
			},
			expected: false,
		},
		{
			name: "层级超出范围",
			info: &ParsedInfo{
				Code:  "1-01-01",
				Name:  "测试分类",
				Level: 5,
			},
			expected: false,
		},
		{
			name: "负层级",
			info: &ParsedInfo{
				Code:  "1-01-01",
				Name:  "测试分类",
				Level: -1,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.info.IsValid()
			if result != tt.expected {
				t.Errorf("IsValid() = %v, expected %v", result, tt.expected)
			}
		})
	}
}