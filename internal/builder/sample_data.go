package builder

import "github.com/freedkr/moonshot/internal/model"

// SampleParsedInfo 测试用的解析数据
var SampleParsedInfo = []*model.ParsedInfo{
	{
		Code:    "1",
		GbmCode: "10000",
		Name:    "国家机关、党群组织、企业、事业单位负责人",
		Level:   0,
	},
	{
		Code:    "1-01",
		GbmCode: "10100",
		Name:    "中国共产党机关负责人",
		Level:   1,
	},
	{
		Code:    "1-01-01",
		GbmCode: "10101",
		Name:    "中国共产党中央委员会和地方各级委员会负责人",
		Level:   2,
	},
	{
		Code:    "1-01-01-01",
		GbmCode: "",
		Name:    "中国共产党中央委员会负责人",
		Level:   3,
	},
	{
		Code:    "1-01-01-02",
		GbmCode: "",
		Name:    "中国共产党地方各级委员会负责人",
		Level:   3,
	},
	{
		Code:    "2",
		GbmCode: "20000",
		Name:    "专业技术人员",
		Level:   0,
	},
	{
		Code:    "2-01",
		GbmCode: "20100",
		Name:    "科学研究人员",
		Level:   1,
	},
}

// SampleCategories 测试用的分类数据
var SampleCategories = []*model.Category{
	{
		Code:    "1",
		GbmCode: "10000",
		Name:    "国家机关、党群组织、企业、事业单位负责人",
		Level:   "大类",
		Children: []*model.Category{
			{
				Code:    "1-01",
				GbmCode: "10100",
				Name:    "中国共产党机关负责人",
				Level:   "中类",
				Children: []*model.Category{
					{
						Code:    "1-01-01",
						GbmCode: "10101",
						Name:    "中国共产党中央委员会和地方各级委员会负责人",
						Level:   "小类",
						Children: []*model.Category{
							{
								Code:  "1-01-01-01",
								Name:  "中国共产党中央委员会负责人",
								Level: "细类",
							},
							{
								Code:  "1-01-01-02",
								Name:  "中国共产党地方各级委员会负责人",
								Level: "细类",
							},
						},
					},
				},
			},
		},
	},
	{
		Code:    "2",
		GbmCode: "20000",
		Name:    "专业技术人员",
		Level:   "大类",
		Children: []*model.Category{
			{
				Code:    "2-01",
				GbmCode: "20100",
				Name:    "科学研究人员",
				Level:   "中类",
			},
		},
	},
}

// InvalidParsedInfo 无效的解析数据，用于测试错误处理
var InvalidParsedInfo = []*model.ParsedInfo{
	{
		Code:  "",
		Name:  "无效数据 - 缺少编码",
		Level: 0,
	},
	{
		Code:  "invalid-code",
		Name:  "",
		Level: -1,
	},
}

// SampleExcelRows 模拟Excel行数据
var SampleExcelRows = [][]string{
	{"1(GBM 10000)国家机关、党群组织、企业、事业单位负责人", "", "", "", "", ""},
	{"", "1-01(GBM 10100)中国共产党机关负责人", "", "", "", ""},
	{"", "", "1-01-01(GBM 10101)中国共产党中央委员会和地方各级委员会负责人", "", "", ""},
	{"", "", "", "", "1-01-01-01\n1-01-01-02", "中国共产党中央委员会负责人\n中国共产党地方各级委员会负责人"},
	{"2(GBM 20000)专业技术人员", "", "", "", "", ""},
	{"", "2-01(GBM 20100)科学研究人员", "", "", "", ""},
}

// SampleJunkRows 垃圾行数据
var SampleJunkRows = [][]string{
	{"大类", "中类", "小类", "细类", "", ""},
	{"续表", "", "", "", "", ""},
	{"", "中华人民共和国职业分类大典", "", "", "", ""},
	{"", "", "分类体系表", "", "", ""},
}
