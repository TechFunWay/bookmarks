#!/bin/bash

# 飞牛应用打包脚本
# 用于为 techfunway.bookmarks 应用创建适用于不同平台的 FnApp 安装包

set -e

echo "=== 飞牛应用打包工具 ==="

# 获取版本号
# 使用 awk 更准确地提取版本号
VERSION=$(awk -F'"' '/appVersion = "[^"]+"/ {print $2}' main.go)

if [ -z "$VERSION" ]; then
    echo "错误: 无法从 main.go 中获取版本号"
    exit 1
fi

echo "使用版本号: $VERSION"

# 定义平台列表
PLATFORMS=("amd64" "arm64")

# 如果指定了平台参数，只打包指定平台
if [ $# -gt 0 ]; then
    if [[ "${PLATFORMS[@]}" =~ "$1" ]]; then
        PLATFORMS=($1)
        echo "只打包平台: $1"
    else
        echo "错误: 不支持的平台: $1"
        echo "支持的平台: ${PLATFORMS[@]}"
        exit 1
    fi
fi

# 主打包函数
package_platform() {
    local ARCH=$1
    local PLATFORM_DIR=""
    local MANIFEST_PLATFORM=""
    local OUTPUT_ARCH=""

    case $ARCH in
        "amd64")
            PLATFORM_DIR="release/bookmarks-v${VERSION}-linux-amd64"
            MANIFEST_PLATFORM="x86"
            OUTPUT_ARCH="amd64"
            ;;
        "arm64")
            PLATFORM_DIR="release/bookmarks-v${VERSION}-linux-arm64"
            MANIFEST_PLATFORM="arm"
            OUTPUT_ARCH="arm64"
            ;;
        *)
            echo "错误: 未知架构: $ARCH"
            return 1
            ;;
    esac

    echo "\n=== 开始打包 $ARCH 平台 ==="

    # 检查平台目录是否存在
    if [ ! -d "$PLATFORM_DIR" ]; then
        echo "错误: 平台目录不存在: $PLATFORM_DIR"
        echo "请先执行编译脚本生成对应平台的可执行文件"
        return 1
    fi

    # 检查可执行文件是否存在
    if [ ! -f "$PLATFORM_DIR/bookmarks" ]; then
        echo "错误: 可执行文件不存在: $PLATFORM_DIR/bookmarks"
        return 1
    fi

    # 检查 static 目录是否存在
    if [ ! -d "$PLATFORM_DIR/static" ]; then
        echo "错误: static 目录不存在: $PLATFORM_DIR/static"
        return 1
    fi

    # 目标目录
    TARGET_DIR="techfunway.bookmarks/app/server"

    # 创建目标目录
    echo "创建目标目录..."
    mkdir -p "$TARGET_DIR"

    # 复制可执行文件
    echo "复制可执行文件..."
    cp "$PLATFORM_DIR/bookmarks" "$TARGET_DIR/"
    chmod +x "$TARGET_DIR/bookmarks"

    # 复制 static 目录
    echo "复制 static 目录..."
    if [ -d "$TARGET_DIR/static" ]; then
        rm -rf "$TARGET_DIR/static"
    fi
    cp -r "$PLATFORM_DIR/static" "$TARGET_DIR/"

    # 修改 manifest 文件
    echo "修改 manifest 文件..."
    MANIFEST_FILE="techfunway.bookmarks/manifest"

    if [ ! -f "$MANIFEST_FILE" ]; then
        echo "错误: manifest 文件不存在: $MANIFEST_FILE"
        return 1
    fi

    # 备份 manifest 文件
    cp "$MANIFEST_FILE" "$MANIFEST_FILE.bak"

    # 修改版本号
    sed -i '' "s/^version               = .*/version               = $VERSION/" "$MANIFEST_FILE"

    # 修改 platform 字段
    sed -i '' "s/^platform              = .*/platform              = $MANIFEST_PLATFORM/" "$MANIFEST_FILE"

    # 更新 desc 字段
    # 根据应用的实际功能，更新 desc 字段的内容
    NEW_DESC="网址收藏夹，收藏自己的网址功能，支持点击跳转<br>1、支持多级文件夹，文件夹下添加网址<br>2、支持自动识别网址名称和网址图标,支持排序和增删改查<br>3、支持搜索功能(支持按网址名称搜索)<br>4、支持导入导出html格式的浏览器书签<br>5、支持手机端<br>6、支持自定义端口<br>7、支持设置背景色、背景图片、面板透明度<br>8、PC端可以设置每行显示数量<br>9、能设置是否显示网址url地址<br>10、支持批量更新信息（名称和图标）<br>11、支持网址图标以文件形式保存到本地，提升部分设备数据加载卡顿问题<br>12、支持导出数据时，图片以base64形式导出，增加导出进度显示和导出结果显示"
    sed -i '' "s/^desc                  = .*/desc                  = $NEW_DESC/" "$MANIFEST_FILE"

    # 从 git 记录中提取 changelog 信息
    echo "从 git 记录中提取 changelog 信息..."
    
    # 初始化分类数组
    new_features=()
    bug_fixes=()
    optimizations=()
    updates=()
    
    # 获取提交记录
    COMMITS=$(git log --oneline -5 | grep -v "版本：" | grep -v "暂存：" | head -4)
    
    if [ -n "$COMMITS" ]; then
        # 处理每个提交记录
        IFS=$'\n'
        commit_array=($COMMITS)
        unset IFS
        
        for commit in "${commit_array[@]}"; do
            # 移除提交哈希和分支信息
            message=$(echo "$commit" | sed 's/^[0-9a-fA-F]\{7\} //' | sed 's/ (HEAD.*)//')
            
            # 分类处理
            classified=false
            
            # 检查关键词
            if [[ "$message" == *"新增"* ]]; then
                new_features+=("$message")
                classified=true
            elif [[ "$message" == *"修复"* ]]; then
                bug_fixes+=("$message")
                classified=true
            elif [[ "$message" == *"优化"* ]]; then
                optimizations+=("$message")
                classified=true
            elif [[ "$message" == *"更新"* ]]; then
                updates+=("$message")
                classified=true
            fi
            
            # 如果没有分类，默认归类为更新
            if [ "$classified" = false ]; then
                updates+=("$message")
            fi
        done
    else
        # 如果 git 记录中没有合适的信息，使用默认的 changelog
        new_features+=("网址图标以文件形式保存到本地，提升部分设备数据加载卡顿问题")
        new_features+=("导出数据时，图片以base64形式导出，增加导出进度显示和导出结果显示")
        optimizations+=("前端查询逻辑，没有根节点的数据查不到问题")
        bug_fixes+=("批量删除只能删除一条数据问题")
    fi
    
    # 生成结构化的 changelog
    GIT_CHANGELOG=""
    section_count=0
    
    # 生成新增内容
    if [ ${#new_features[@]} -gt 0 ]; then
        if [ $section_count -gt 0 ]; then
            GIT_CHANGELOG+="<br>"
        fi
        GIT_CHANGELOG+="<strong>新增：</strong><br>"
        for i in "${!new_features[@]}"; do
            item=${new_features[$i]}
            item=$(echo "$item" | sed "s/^新增：//")
            GIT_CHANGELOG+="$((i+1))、$item<br>"
        done
        section_count=$((section_count+1))
    fi
    
    # 生成修复内容
    if [ ${#bug_fixes[@]} -gt 0 ]; then
        if [ $section_count -gt 0 ]; then
            GIT_CHANGELOG+="<br>"
        fi
        GIT_CHANGELOG+="<strong>修复：</strong><br>"
        for i in "${!bug_fixes[@]}"; do
            item=${bug_fixes[$i]}
            item=$(echo "$item" | sed "s/^修复：//")
            GIT_CHANGELOG+="$((i+1))、$item<br>"
        done
        section_count=$((section_count+1))
    fi
    
    # 生成优化内容
    if [ ${#optimizations[@]} -gt 0 ]; then
        if [ $section_count -gt 0 ]; then
            GIT_CHANGELOG+="<br>"
        fi
        GIT_CHANGELOG+="<strong>优化：</strong><br>"
        for i in "${!optimizations[@]}"; do
            item=${optimizations[$i]}
            item=$(echo "$item" | sed "s/^优化：//")
            GIT_CHANGELOG+="$((i+1))、$item<br>"
        done
        section_count=$((section_count+1))
    fi
    
    # 生成更新内容
    if [ ${#updates[@]} -gt 0 ]; then
        if [ $section_count -gt 0 ]; then
            GIT_CHANGELOG+="<br>"
        fi
        GIT_CHANGELOG+="<strong>更新：</strong><br>"
        for i in "${!updates[@]}"; do
            item=${updates[$i]}
            item=$(echo "$item" | sed "s/^更新：//")
            GIT_CHANGELOG+="$((i+1))、$item<br>"
        done
        section_count=$((section_count+1))
    fi
    
    # 生成总结
    summary="本次更新"
    has_changes=false
    
    # 检查新增
    if [ ${#new_features[@]} -gt 0 ]; then
        if [ "$has_changes" = true ]; then
            summary+="，"
        fi
        summary+="${#new_features[@]}项新增"
        has_changes=true
    fi
    
    # 检查修复
    if [ ${#bug_fixes[@]} -gt 0 ]; then
        if [ "$has_changes" = true ]; then
            summary+="，"
        fi
        summary+="${#bug_fixes[@]}项修复"
        has_changes=true
    fi
    
    # 检查优化
    if [ ${#optimizations[@]} -gt 0 ]; then
        if [ "$has_changes" = true ]; then
            summary+="，"
        fi
        summary+="${#optimizations[@]}项优化"
        has_changes=true
    fi
    
    # 检查更新
    if [ ${#updates[@]} -gt 0 ]; then
        if [ "$has_changes" = true ]; then
            summary+="，"
        fi
        summary+="${#updates[@]}项更新"
        has_changes=true
    fi
    
    if [ "$has_changes" = false ]; then
        summary="本次更新无具体修改内容"
    fi
    
    # 构建最终的 changelog
    NEW_CHANGELOG="<strong>版本更新总结：</strong>$summary<br><br>$GIT_CHANGELOG<br><strong>⚠️注意：如果升级失败，请先保留数据卸载后重装<strong>"
    
    # 使用更兼容的方式修改 manifest 文件
    # 1. 先删除现有的 changelog 行
    sed -i '' '/^changelog             = .*/d' "$MANIFEST_FILE"
    
    # 2. 在 maintainer_url 后添加新的 changelog
    # 兼容 BSD sed (macOS) 和 GNU sed
    awk -v changelog="$NEW_CHANGELOG" '/^maintainer_url        = .*/ {print; print "changelog             = " changelog; next} {print}' "$MANIFEST_FILE" > "$MANIFEST_FILE.tmp"
    mv "$MANIFEST_FILE.tmp" "$MANIFEST_FILE"

    # 进入 techfunway.bookmarks 目录执行打包
    echo "执行应用打包..."
    cd "techfunway.bookmarks"

    # 执行 fnpack build 命令
    if command -v fnpack &> /dev/null; then
        fnpack build
    else
        echo "错误: fnpack 命令未找到，请确保已安装飞牛应用打包工具"
        cd ..
        return 1
    fi

    cd ..

    # 检查打包结果
    if [ ! -f "techfunway.bookmarks/techfunway.bookmarks.fpk" ]; then
        echo "错误: 打包失败，未生成 fpk 文件"
        return 1
    fi

    # 确保 release 目录存在
    mkdir -p "release"

    # 重命名打包文件并移动到 release 目录
    OUTPUT_FILE="techfunway.bookmarks-v${VERSION}-${OUTPUT_ARCH}.fpk"
    RELEASE_OUTPUT="release/$OUTPUT_FILE"
    echo "重命名打包文件为: $OUTPUT_FILE"
    echo "移动到 release 目录"
    mv "techfunway.bookmarks/techfunway.bookmarks.fpk" "$RELEASE_OUTPUT"

    echo "平台 $ARCH 打包完成！"
    echo "打包文件位置: $(pwd)/$RELEASE_OUTPUT"
}

# 执行打包
for ARCH in "${PLATFORMS[@]}"; do
    package_platform "$ARCH"
    echo ""
done

echo "=== 打包完成 ==="
echo "生成的文件:"
ls -la release/techfunway.bookmarks-v${VERSION}-*.fpk || true

echo "\n使用说明:"
echo "1. 打包文件已生成在 release 目录"
echo "2. 可以直接将这些文件上传到飞牛应用商店"
echo "3. 或者分发给用户手动安装"
