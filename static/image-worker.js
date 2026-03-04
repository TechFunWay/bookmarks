// Web Worker for image conversion
self.onmessage = function(e) {
  const { type, data } = e.data;
  
  if (type === 'convertImage') {
    convertImageToBase64(data);
  }
};

async function convertImageToBase64(data) {
  const { url, index, total } = data;
  
  try {
    // 如果已经是base64格式，直接返回
    if (url.startsWith('data:')) {
      self.postMessage({
        type: 'progress',
        index,
        total,
        success: true,
        result: url,
        message: '图片已是base64格式'
      });
      return;
    }
    
    // 处理本地路径
    let imageUrl = url;
    if (url.startsWith('/') && !url.startsWith('//')) {
      imageUrl = self.location.origin + url;
    }
    
    // 使用fetch获取图片
    const response = await fetch(imageUrl, {
      mode: 'cors',
      credentials: 'omit'
    });
    
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    
    const blob = await response.blob();
    const reader = new FileReaderSync();
    const base64 = reader.readAsDataURL(blob);
    
    self.postMessage({
      type: 'progress',
      index,
      total,
      success: true,
      result: base64,
      message: '图片转换成功'
    });
  } catch (error) {
    self.postMessage({
      type: 'progress',
      index,
      total,
      success: false,
      result: url,
      message: `图片转换失败: ${error.message}`
    });
  }
}