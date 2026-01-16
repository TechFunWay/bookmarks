# 插件图标

本目录包含插件所需的图标文件。由于SVG格式的图标在浏览器扩展中可能不被完全支持，建议将SVG转换为PNG格式。

## 图标尺寸

需要以下尺寸的PNG图标：
- icon16.png (16x16)
- icon32.png (32x32)
- icon48.png (48x48)
- icon128.png (128x128)

## 转换方法

1. 使用在线SVG转PNG工具，如：
   - https://convertio.co/zh/svg-png/
   - https://svgtopng.com/

2. 使用图像编辑软件，如：
   - Adobe Illustrator
   - Inkscape (免费)
   - GIMP (免费)

3. 使用命令行工具，如：
   ```bash
   # 使用imagemagick
   convert icon16.svg icon16.png
   convert icon32.svg icon32.png
   convert icon48.svg icon48.png
   convert icon128.svg icon128.png
   ```

## 注意事项

- 确保转换后的PNG图标清晰可见
- 保持图标的颜色和形状与SVG一致
- 转换后删除SVG文件或将其备份到其他位置
