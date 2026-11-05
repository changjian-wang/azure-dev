"use strict";
async function decode(event) {
    let image;
    try {
        const data = event.data;
        if (!data || typeof data !== "object" || !("bytes" in data) ||
            !(data.bytes instanceof ArrayBuffer) || data.bytes.byteLength === 0) {
            throw new Error("Nonempty owned HEIF bytes are required.");
        }
        try {
            importScripts(new URL("./heic_decoder.js", self.location.href).href);
        }
        catch (error) {
            throw new Error("The HEIF decoder script could not be loaded.", { cause: error });
        }
        if (typeof wasm_bindgen !== "function" || typeof wasm_bindgen.decode_heic !== "function") {
            throw new Error("The HEIF decoder factory is unavailable.");
        }
        await wasm_bindgen(new URL("./heic_decoder_bg.wasm", self.location.href));
        image = wasm_bindgen.decode_heic(new Uint8Array(data.bytes), 40_000_000);
        const width = image.width;
        const height = image.height;
        if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width <= 0 || height <= 0 ||
            !Number.isSafeInteger(width * height * 4)) {
            throw new Error("The decoded HEIF image has invalid dimensions.");
        }
        const pixels = image.take_rgba();
        image = undefined;
        if (pixels.byteLength !== width * height * 4)
            throw new Error("The HEIF decoder returned an invalid pixel buffer.");
        const rgba = pixels.buffer.slice(pixels.byteOffset, pixels.byteOffset + pixels.byteLength);
        self.postMessage({ type: "decoded", width, height, rgba }, [rgba]);
    }
    catch (error) {
        self.postMessage({ type: "error", message: error instanceof Error ? error.message : String(error) });
    }
    finally {
        image?.free();
    }
}
self.addEventListener("message", (event) => {
    void decode(event).catch((error) => {
        self.postMessage({ type: "error", message: error instanceof Error ? error.message : String(error) });
    });
});
