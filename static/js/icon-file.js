// 自定义图标上传的公共处理：本地图片 → 居中裁方 → 缩放到目标边长 → PNG dataURL。
// pc / mobile / admin 三个页面共用；上传接口是 POST /api/icons/upload。
window.bookmarksIconFile = {
  // 把图片文件读成 512×512（默认）的 PNG data URL，供预览与上传
  toDataUrl(file, size) {
    size = size || 512;
    return new Promise((resolve, reject) => {
      if (!file || !/^image\//.test(file.type)) {
        reject(new Error('请选择图片文件'));
        return;
      }
      const reader = new FileReader();
      reader.onerror = () => reject(new Error('读取图片失败'));
      reader.onload = () => {
        const img = new Image();
        img.onerror = () => reject(new Error('图片解析失败，请换一张试试'));
        img.onload = () => {
          const side = Math.min(img.naturalWidth, img.naturalHeight);
          if (!side) {
            reject(new Error('图片内容为空'));
            return;
          }
          const sx = (img.naturalWidth - side) / 2;
          const sy = (img.naturalHeight - side) / 2;
          const canvas = document.createElement('canvas');
          canvas.width = size;
          canvas.height = size;
          canvas.getContext('2d').drawImage(img, sx, sy, side, side, 0, 0, size, size);
          resolve(canvas.toDataURL('image/png'));
        };
        img.src = reader.result;
      };
      reader.readAsDataURL(file);
    });
  },

  // 压缩并上传，返回后端保存后的本地图标路径（/icons/...）
  async upload(file, headers, size) {
    const dataUrl = await this.toDataUrl(file, size);
    const res = await fetch('/api/icons/upload', {
      method: 'POST',
      headers: headers,
      body: JSON.stringify({ data: dataUrl })
    });
    if (!res.ok) {
      const err = await res.json().catch(() => ({}));
      throw new Error(err.error || '上传失败');
    }
    const out = await res.json();
    return out.url;
  }
};
