#!/bin/bash
# 快速启动脚本 - 为开发者提供最简单的启动方式
# 这是 deploy.sh 的简化封装，专门用于本地开发环境的快速启动

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "====================================="
echo "🚀 Moonshot 快速启动 (开发环境)"
echo "====================================="
echo ""
echo "这个脚本会："
echo "1. 清理旧的数据卷（如果选择）"
echo "2. 启动所有基础服务"
echo "3. 自动初始化数据库（包括750KB的分类数据）"
echo "4. 显示服务访问地址"
echo ""

# 询问是否清理旧数据
read -p "是否清理旧数据并重新初始化？[y/N]: " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "正在清理旧数据..."
    "$SCRIPT_DIR/deploy.sh" --env local --action down
    # 清理数据卷
    docker volume rm moonshot_postgres_data 2>/dev/null || true
    echo "✅ 旧数据已清理"
fi

# 使用 deploy.sh 启动服务
echo "正在启动所有服务..."
"$SCRIPT_DIR/deploy.sh" --env local --services all --action up

echo ""
echo "====================================="
echo "✅ 快速启动完成！"
echo "====================================="
echo ""
echo "数据库已自动初始化，包含："
echo "  - 所有表结构 (moonshot.sql)"
echo "  - 分类数据 (categories.sql, 750KB)"
echo ""
echo "💡 提示：首次启动可能需要1-2分钟完成数据导入"
echo ""