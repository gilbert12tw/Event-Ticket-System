import { expect, type APIRequestContext, type Locator } from "@playwright/test";
import { deflateSync } from "node:zlib";
import type { Actor } from "./live-flow-helpers";
import { apiResponse } from "./live-flow-helpers";

export type PosterFixture = {
  name: string;
  mimeType: "image/png";
  buffer: Buffer;
  rgb: readonly [number, number, number];
  width: number;
  height: number;
};

const posterWidth = 16;
const posterHeight = 10;

export function createPosterFixture(
  name: string,
  rgb: readonly [number, number, number],
): PosterFixture {
  return {
    name,
    mimeType: "image/png",
    buffer: createSolidPng(posterWidth, posterHeight, rgb),
    rgb,
    width: posterWidth,
    height: posterHeight,
  };
}

export function posterUploadFile(poster: PosterFixture) {
  return {
    name: poster.name,
    mimeType: poster.mimeType,
    buffer: poster.buffer,
  };
}

export async function expectPosterAPI(
  request: APIRequestContext,
  actor: Actor,
  eventID: string,
  poster: PosterFixture,
) {
  const response = await apiResponse(
    request,
    actor,
    "GET",
    `/api/v1/events/${encodeURIComponent(eventID)}/poster`,
  );
  const body = await response.body();
  expect(response.ok(), `GET poster failed: ${await response.text()}`).toBe(
    true,
  );
  expect(response.headers()["content-type"]).toContain("image/png");
  expect(
    body.equals(poster.buffer),
    `poster API bytes should match ${poster.name}`,
  ).toBe(true);
}

export async function expectPosterRendered(
  posterFrame: Locator,
  poster: PosterFixture,
  label: string,
) {
  await posterFrame.scrollIntoViewIfNeeded();
  await expect(
    posterFrame.locator(".employee-event-poster-fallback"),
    `${label}: fallback must not render`,
  ).toHaveCount(0);

  const image = posterFrame.locator("img").first();
  await expect(image, `${label}: poster image`).toBeVisible();
  await expect
    .poll(
      () =>
        image.evaluate((element) => {
          const img = element as HTMLImageElement;
          return img.complete && img.naturalWidth > 2 && img.naturalHeight > 2;
        }),
      { message: `${label}: poster image loaded` },
    )
    .toBe(true);

  const sample = await image.evaluate((element) => {
    const img = element as HTMLImageElement;
    const canvas = document.createElement("canvas");
    canvas.width = img.naturalWidth;
    canvas.height = img.naturalHeight;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("canvas context unavailable");
    context.drawImage(img, 0, 0);
    const [r, g, b, a] = context.getImageData(
      Math.floor(canvas.width / 2),
      Math.floor(canvas.height / 2),
      1,
      1,
    ).data;
    return {
      a,
      b,
      g,
      naturalHeight: img.naturalHeight,
      naturalWidth: img.naturalWidth,
      r,
    };
  });

  expect(sample.naturalWidth, `${label}: natural width`).toBe(poster.width);
  expect(sample.naturalHeight, `${label}: natural height`).toBe(poster.height);
  expect(sample.a, `${label}: alpha`).toBe(255);
  expect(
    Math.max(
      Math.abs(sample.r - poster.rgb[0]),
      Math.abs(sample.g - poster.rgb[1]),
      Math.abs(sample.b - poster.rgb[2]),
    ),
    `${label}: sampled poster pixel should match ${poster.name}`,
  ).toBeLessThanOrEqual(3);
}

function createSolidPng(
  width: number,
  height: number,
  [r, g, b]: readonly [number, number, number],
) {
  const signature = Buffer.from([
    0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
  ]);
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(width, 0);
  ihdr.writeUInt32BE(height, 4);
  ihdr[8] = 8;
  ihdr[9] = 6;
  ihdr[10] = 0;
  ihdr[11] = 0;
  ihdr[12] = 0;

  return Buffer.concat([
    signature,
    pngChunk("IHDR", ihdr),
    pngChunk("IDAT", deflateSync(solidScanlines(width, height, [r, g, b]))),
    pngChunk("IEND", Buffer.alloc(0)),
  ]);
}

function solidScanlines(
  width: number,
  height: number,
  [r, g, b]: readonly [number, number, number],
) {
  const rowLength = 1 + width * 4;
  const body = Buffer.alloc(rowLength * height);
  for (let y = 0; y < height; y += 1) {
    const row = y * rowLength;
    body[row] = 0;
    for (let x = 0; x < width; x += 1) {
      const index = row + 1 + x * 4;
      body[index] = r;
      body[index + 1] = g;
      body[index + 2] = b;
      body[index + 3] = 255;
    }
  }
  return body;
}

function pngChunk(type: string, data: Buffer) {
  const typeBuffer = Buffer.from(type, "ascii");
  const length = Buffer.alloc(4);
  const crc = Buffer.alloc(4);
  length.writeUInt32BE(data.length, 0);
  crc.writeUInt32BE(crc32(Buffer.concat([typeBuffer, data])), 0);
  return Buffer.concat([length, typeBuffer, data, crc]);
}

const crcTable = Array.from({ length: 256 }, (_, index) => {
  let value = index;
  for (let bit = 0; bit < 8; bit += 1) {
    value = value & 1 ? 0xedb88320 ^ (value >>> 1) : value >>> 1;
  }
  return value >>> 0;
});

function crc32(buffer: Buffer) {
  let crc = 0xffffffff;
  for (const byte of buffer) {
    crc = crcTable[(crc ^ byte) & 0xff] ^ (crc >>> 8);
  }
  return (crc ^ 0xffffffff) >>> 0;
}
