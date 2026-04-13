/******************************************************************************
 * Spine Runtimes License Agreement
 * Last updated July 28, 2023. Replaces all prior versions.
 *
 * Copyright (c) 2013-2023, Esoteric Software LLC
 *
 * Integration of the Spine Runtimes into software or otherwise creating
 * derivative works of the Spine Runtimes is permitted under the terms and
 * conditions of Section 2 of the Spine Editor License Agreement:
 * http://esotericsoftware.com/spine-editor-license
 *
 * Otherwise, it is permitted to integrate the Spine Runtimes into software or
 * otherwise create derivative works of the Spine Runtimes (collectively,
 * "Products"), provided that each user of the Products must obtain their own
 * Spine Editor license and redistribution of the Products in any form must
 * include this license and copyright notice.
 *
 * THE SPINE RUNTIMES ARE PROVIDED BY ESOTERIC SOFTWARE LLC "AS IS" AND ANY
 * EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
 * WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
 * DISCLAIMED. IN NO EVENT SHALL ESOTERIC SOFTWARE LLC BE LIABLE FOR ANY
 * DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES
 * (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES,
 * BUSINESS INTERRUPTION, OR LOSS OF USE, DATA, OR PROFITS) HOWEVER CAUSED AND
 * ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF THE
 * SPINE RUNTIMES, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *****************************************************************************/

export * from "@esotericsoftware/spine-core";

import {
	AnimationState,
	AnimationStateData,
	AtlasAttachmentLoader,
	BlendMode,
	ClippingAttachment,
	type Color,
	MathUtils,
	MeshAttachment,
	type NumberArrayLike,
	Physics,
	RegionAttachment,
	Skeleton,
	SkeletonBinary,
	SkeletonClipping,
	type SkeletonData,
	SkeletonJson,
	Texture,
	TextureAtlas,
	Utils,
} from "@esotericsoftware/spine-core";

import type { Canvas, CanvasKit, Image, Paint, Shader } from "canvaskit-wasm";

Skeleton.yDown = true;

type CanvasKitImage = {
	shaders: Shader[];
	paintPerBlendMode: Map<BlendMode, Paint>;
	image: Image;
};

// CanvasKit blend modes for premultiplied alpha
function toCkBlendMode (ck: CanvasKit, blendMode: BlendMode) {
	switch (blendMode) {
		case BlendMode.Normal:
			return ck.BlendMode.SrcOver;
		case BlendMode.Additive:
			return ck.BlendMode.Plus;
		case BlendMode.Multiply:
			return ck.BlendMode.SrcOver;
		case BlendMode.Screen:
			return ck.BlendMode.Screen;
		default:
			return ck.BlendMode.SrcOver;
	}
}

function bufferToUtf8String (buffer: ArrayBuffer | Buffer) {
	if (typeof Buffer !== "undefined") {
		return buffer.toString("utf-8");
	} else if (typeof TextDecoder !== "undefined") {
		return new TextDecoder("utf-8").decode(buffer);
	} else {
		throw new Error("Unsupported environment");
	}
}

class CanvasKitTexture extends Texture {
	getImage (): CanvasKitImage {
		return this._image;
	}

	setFilters (): void { }

	setWraps (): void { }

	dispose (): void {
		const data: CanvasKitImage = this._image;
		for (const paint of data.paintPerBlendMode.values()) {
			paint.delete();
		}
		for (const shader of data.shaders) {
			shader.delete();
		}
		data.image.delete();
		this._image = null;
	}

	static async fromFile (
		ck: CanvasKit,
		path: string,
		readFile: (path: string) => Promise<ArrayBuffer | Buffer>
	): Promise<CanvasKitTexture> {
		const imgData = await readFile(path);
		if (!imgData) throw new Error(`Could not load image ${path}`);
		const image = ck.MakeImageFromEncoded(imgData);
		if (!image) throw new Error(`Could not load image ${path}`);
		const paintPerBlendMode = new Map<BlendMode, Paint>();
		const shaders: Shader[] = [];
		for (const blendMode of [
			BlendMode.Normal,
			BlendMode.Additive,
			BlendMode.Multiply,
			BlendMode.Screen,
		]) {
			const paint = new ck.Paint();
			const shader = image.makeShaderOptions(
				ck.TileMode.Clamp,
				ck.TileMode.Clamp,
				ck.FilterMode.Linear,
				ck.MipmapMode.Linear
			);
			paint.setShader(shader);
			paint.setBlendMode(toCkBlendMode(ck, blendMode));
			paintPerBlendMode.set(blendMode, paint);
			shaders.push(shader);
		}
		return new CanvasKitTexture({ shaders, paintPerBlendMode, image });
	}
}

/**
 * Loads a {@link TextureAtlas} and its atlas page images from the given file path using the `readFile(path: string): Promise<Buffer>` function.
 * Throws an `Error` if the file or one of the atlas page images could not be loaded.
 */
export async function loadTextureAtlas (
	ck: CanvasKit,
	atlasFile: string,
	readFile: (path: string) => Promise<ArrayBuffer | Buffer>
): Promise<TextureAtlas> {
	const atlas = new TextureAtlas(bufferToUtf8String(await readFile(atlasFile)));
	const slashIndex = atlasFile.lastIndexOf("/");
	const parentDir =
		slashIndex >= 0 ? atlasFile.substring(0, slashIndex + 1) : "";
	for (const page of atlas.pages) {
		const texture = await CanvasKitTexture.fromFile(
			ck,
			parentDir + page.name,
			readFile
		);
		page.setTexture(texture);
	}
	return atlas;
}

/**
 * Loads a {@link SkeletonData} from the given file path (`.json` or `.skel`) using the `readFile(path: string): Promise<Buffer>` function.
 * Attachments will be looked up in the provided atlas.
 */
export async function loadSkeletonData (
	skeletonFile: string,
	atlas: TextureAtlas,
	readFile: (path: string) => Promise<ArrayBuffer | Buffer>,
	scale = 1
): Promise<SkeletonData> {
	const attachmentLoader = new AtlasAttachmentLoader(atlas);
	const loader = skeletonFile.endsWith(".json")
		? new SkeletonJson(attachmentLoader)
		: new SkeletonBinary(attachmentLoader);
	loader.scale = scale;
	const data = await readFile(skeletonFile);
	if (loader instanceof SkeletonJson) {
		return loader.readSkeletonData(bufferToUtf8String(data))
	}
	return loader.readSkeletonData(data);
}

/**
 * Manages a {@link Skeleton} and its associated {@link AnimationState}. A drawable is constructed from a {@link SkeletonData}, which can
 * be shared by any number of drawables.
 */
export class SkeletonDrawable {
	public readonly skeleton: Skeleton;
	public readonly animationState: AnimationState;

    /**
     * Constructs a new drawble from the skeleton data.
     */
	constructor (skeletonData: SkeletonData) {
		this.skeleton = new Skeleton(skeletonData);
		this.animationState = new AnimationState(
			new AnimationStateData(skeletonData)
		);
	}

    /**
     * Updates the animation state and skeleton time by the delta time. Applies the
     * animations to the skeleton and calculates the final pose of the skeleton.
     *
     * @param deltaTime the time since the last update in seconds
     * @param physicsUpdate optional {@link Physics} update mode.
     */
	update (deltaTime: number, physicsUpdate: Physics = Physics.update) {
		this.animationState.update(deltaTime);
		this.skeleton.update(deltaTime);
		this.animationState.apply(this.skeleton);
		this.skeleton.updateWorldTransform(physicsUpdate);
	}
}

/**
 * Renders a {@link Skeleton} or {@link SkeletonDrawable} to a CanvasKit {@link Canvas}.
 */
export class SkeletonRenderer {
	private clipper = new SkeletonClipping();
	private static QUAD_TRIANGLES = [0, 1, 2, 2, 3, 0];
	private scratchPositions = Utils.newFloatArray(100);
	private scratchUVs = Utils.newFloatArray(100);
	private scratchColors = new Uint32Array(100 / 4);

    /**
     * Creates a new skeleton renderer.
     * @param ck the {@link CanvasKit} instance returned by `CanvasKitInit()`.
     */
	constructor (private ck: CanvasKit) { }

    /**
     * Renders a skeleton or skeleton drawable in its current pose to the canvas.
     * @param canvas the canvas to render to.
     * @param skeleton the skeleton or drawable to render.
     */
	render (canvas: Canvas, skeleton: Skeleton | SkeletonDrawable) {
		if (skeleton instanceof SkeletonDrawable) skeleton = skeleton.skeleton;
		const clipper = this.clipper;
		const drawOrder = skeleton.drawOrder;
		const skeletonColor = skeleton.color;

		for (let i = 0, n = drawOrder.length; i < n; i++) {
			const slot = drawOrder[i];
			if (!slot.bone.active) {
				clipper.clipEndWithSlot(slot);
				continue;
			}

			const attachment = slot.getAttachment();
			let positions = this.scratchPositions;
			let triangles: Array<number>;
			let numVertices = 4;

			if (attachment instanceof RegionAttachment) {
				attachment.computeWorldVertices(slot, positions, 0, 2);
				triangles = SkeletonRenderer.QUAD_TRIANGLES;
			} else if (attachment instanceof MeshAttachment) {
				if (positions.length < attachment.worldVerticesLength) {
					this.scratchPositions = Utils.newFloatArray(attachment.worldVerticesLength);
					positions = this.scratchPositions;
				}
				numVertices = attachment.worldVerticesLength >> 1;
				attachment.computeWorldVertices(
					slot,
					0,
					attachment.worldVerticesLength,
					positions,
					0,
					2
				);
				triangles = attachment.triangles;
			} else if (attachment instanceof ClippingAttachment) {
				clipper.clipStart(slot, attachment);
				continue;
			} else {
				clipper.clipEndWithSlot(slot);
				continue;
			}

			const texture = attachment.region?.texture as CanvasKitTexture;
			if (texture) {
				let uvs = attachment.uvs;
				let scaledUvs: NumberArrayLike;
				let colors = this.scratchColors;
				if (clipper.isClipping()) {
					clipper.clipTrianglesUnpacked(positions, triangles, triangles.length, uvs);
					if (clipper.clippedVertices.length <= 0) {
						clipper.clipEndWithSlot(slot);
						continue;
					}
					positions = clipper.clippedVertices;
					uvs = scaledUvs = clipper.clippedUVs;
					triangles = clipper.clippedTriangles;
					numVertices = clipper.clippedVertices.length / 2;
					colors = new Uint32Array(numVertices);
				} else {
					scaledUvs = this.scratchUVs;
					if (this.scratchUVs.length < uvs.length)
						scaledUvs = this.scratchUVs = Utils.newFloatArray(uvs.length);
					if (colors.length < numVertices)
						colors = this.scratchColors = new Uint32Array(numVertices);
				}

				const ckImage = texture.getImage();
				const image = ckImage.image;
				const width = image.width();
				const height = image.height();
				for (let i = 0; i < uvs.length; i += 2) {
					scaledUvs[i] = uvs[i] * width;
					scaledUvs[i + 1] = uvs[i + 1] * height;
				}

				const attachmentColor = attachment.color;
				const slotColor = slot.color;

				// using Uint32Array for colors allows to avoid canvaskit to allocate one each time
				// but colors need to be in canvaskit format.
				// See: https://github.com/google/skia/blob/bb8c36fdf7b915a8c096e35e2f08109e477fe1b8/modules/canvaskit/color.js#L163
				const finalColor = (
					MathUtils.clamp(skeletonColor.a * slotColor.a * attachmentColor.a * 255, 0, 255) << 24 |
					MathUtils.clamp(skeletonColor.r * slotColor.r * attachmentColor.r * 255, 0, 255) << 16 |
					MathUtils.clamp(skeletonColor.g * slotColor.g * attachmentColor.g * 255, 0, 255) << 8 |
					MathUtils.clamp(skeletonColor.b * slotColor.b * attachmentColor.b * 255, 0, 255) << 0
				) >>> 0;
				for (let i = 0, n = numVertices; i < n; i++) colors[i] = finalColor;

				const vertices = this.ck.MakeVertices(
					this.ck.VertexMode.Triangles,
					positions,
					scaledUvs,
					colors,
					triangles,
					false
				);
				const ckPaint = ckImage.paintPerBlendMode.get(slot.data.blendMode);
				if (ckPaint) canvas.drawVertices(vertices, this.ck.BlendMode.Modulate, ckPaint);
				vertices.delete();
			}

			clipper.clipEndWithSlot(slot);
		}
		clipper.clipEnd();
	}
}
