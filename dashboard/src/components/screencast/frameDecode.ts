function loadImageElement(blob: Blob): Promise<HTMLImageElement> {
  return new Promise((resolve, reject) => {
    const imgUrl = URL.createObjectURL(blob);
    const img = new Image();

    img.onload = () => {
      URL.revokeObjectURL(imgUrl);
      resolve(img);
    };
    img.onerror = () => {
      URL.revokeObjectURL(imgUrl);
      reject(new Error("Failed to decode screencast frame"));
    };

    img.src = imgUrl;
  });
}

export async function drawFrameToCanvas(
  canvas: HTMLCanvasElement,
  ctx: CanvasRenderingContext2D,
  frameData: ArrayBuffer | Blob,
): Promise<void> {
  const blob =
    frameData instanceof Blob
      ? frameData
      : new Blob([frameData], { type: "image/jpeg" });

  if (typeof window.createImageBitmap === "function") {
    const bitmap = await window.createImageBitmap(blob);
    try {
      canvas.width = bitmap.width;
      canvas.height = bitmap.height;
      ctx.drawImage(bitmap, 0, 0);
      return;
    } finally {
      bitmap.close();
    }
  }

  const image = await loadImageElement(blob);
  canvas.width = image.width;
  canvas.height = image.height;
  ctx.drawImage(image, 0, 0);
}

export { loadImageElement };
