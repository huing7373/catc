/******************************************************************************
 * Spine Runtimes License Agreement
 * Last updated April 5, 2025. Replaces all prior versions.
 *
 * Copyright (c) 2013-2025, Esoteric Software LLC
 *
 * Integration of the Spine Runtimes into software or otherwise creating
 * derivative works of the Spine Runtimes is permitted under the terms and
 * conditions of Section 2 of the Spine Editor License Agreement:
 * http://esotericsoftware.com/spine-editor-license
 *
 * Otherwise, it is permitted to integrate the Spine Runtimes into software
 * or otherwise create derivative works of the Spine Runtimes (collectively,
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
 * (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
 * THE SPINE RUNTIMES, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 *****************************************************************************/

package spine.attachments;

import spine.Color;
import spine.atlas.TextureAtlasRegion;
import spine.atlas.TextureAtlasPage;

/** An attachment that displays a textured mesh. A mesh has hull vertices and internal vertices within the hull. Holes are not
 * supported. Each vertex has UVs (texture coordinates) and triangles are used to map an image on to the mesh.
 *
 * @see https://esotericsoftware.com/spine-meshes Mesh attachments in the Spine User Guide */
class MeshAttachment extends VertexAttachment implements HasTextureRegion {
	public var region:TextureRegion;
	public var path:String;
	/** The UV pair for each vertex, normalized within the texture region. */
	public var regionUVs = new Array<Float>();
	/** The UV pair for each vertex, normalized within the entire texture.
	 * See #updateRegion(). */
	public var uvs = new Array<Float>();
	/** Triplets of vertex indices which describe the mesh's triangulation. */
	public var triangles = new Array<Int>();
	public var color:Color = new Color(1, 1, 1, 1);
	/** The width of the mesh's image, or zero if nonessential data was not exported. */
	public var width:Float = 0;
	/** The height of the mesh's image, or zero if nonessential data was not exported. */
	public var height:Float = 0;
	/** The number of entries at the beginning of #vertices that make up the mesh hull. */
	public var hullLength:Int = 0;
	/** Vertex index pairs describing edges for controlling triangulation, or null if nonessential data was not exported. Mesh
	 * triangles will never cross edges. Triangulation is not performed at runtime. */
	public var edges = new Array<Int>();
	public var rendererObject:Dynamic;
	public var sequence:Sequence;

	/** The parent mesh if this is a linked mesh, else null. A linked mesh shares the #bones, #vertices,
	 * #regionUVs, #triangles, #hullLength, #edges, #width, and #height with the
	 * parent mesh, but may have a different #name or #path (and therefore a different texture). */
	private var _parentMesh:MeshAttachment;

	/** Copy constructor. Use newLinkedMesh() if the other mesh is a linked mesh. */
	public function new(name:String, path:String) {
		super(name);
		this.path = path;
	}

	/** Calculates uvs using the regionUVs and region. Must be called if the region, the region's properties, or
	 * the regionUVs are changed. */
	public function updateRegion():Void {
		if (region == null) {
			throw new SpineException("Region not set.");
			return;
		}
		var regionUVs = this.regionUVs;
		if (uvs.length != regionUVs.length) {
			uvs = new Array<Float>();
			uvs.resize(regionUVs.length);
		}
		var n = uvs.length;
		var u = region.u, v = region.v, width:Float = 0, height:Float = 0;
		if (Std.isOfType(region, TextureAtlasRegion)) {
			var atlasRegion:TextureAtlasRegion = cast(region, TextureAtlasRegion), page:TextureAtlasPage = atlasRegion.page;
			var textureWidth = page.width, textureHeight = page.height;
			switch (atlasRegion.degrees) {
				case 90:
					u -= (region.originalHeight - region.offsetY - region.height) / textureWidth;
					v -= (region.originalWidth - region.offsetX - region.width) / textureHeight;
					width = region.originalHeight / textureWidth;
					height = region.originalWidth / textureHeight;
					var i = 0;
					while (i < n) {
						uvs[i] = u + regionUVs[i + 1] * width;
						uvs[i + 1] = v + (1 - regionUVs[i]) * height;
						i += 2;
					}
					return;
				case 180:
					u -= (region.originalWidth - region.offsetX - region.width) / textureWidth;
					v -= region.offsetY / textureHeight;
					width = region.originalWidth / textureWidth;
					height = region.originalHeight / textureHeight;
					var i = 0;
					while (i < n) {
						uvs[i] = u + (1 - regionUVs[i]) * width;
						uvs[i + 1] = v + (1 - regionUVs[i + 1]) * height;
						i += 2;
					}
					return;
				case 270:
					u -= region.offsetY / textureWidth;
					v -= region.offsetX / textureHeight;
					width = region.originalHeight / textureWidth;
					height = region.originalWidth / textureHeight;
					var i = 0;
					while (i < n) {
						uvs[i] = u + (1 - regionUVs[i + 1]) * width;
						uvs[i + 1] = v + regionUVs[i] * height;
						i += 2;
					}
					return;
			}
			u -= region.offsetX / textureWidth;
			v -= (region.originalHeight - region.offsetY - region.height) / textureHeight;
			width = region.originalWidth / textureWidth;
			height = region.originalHeight / textureHeight;
		} else if (region == null) {
			u = v = 0;
			width = height = 1;
		} else {
			width = this.region.u2 - u;
			height = this.region.v2 - v;
		}
		var i = 0;
		while (i < n) {
			uvs[i] = u + regionUVs[i] * width;
			uvs[i + 1] = v + regionUVs[i + 1] * height;
			i += 2;
		}
	}

	public var parentMesh(get, set):MeshAttachment;

	private function get_parentMesh():MeshAttachment {
		return _parentMesh;
	}

	private function set_parentMesh(parentMesh:MeshAttachment):MeshAttachment {
		_parentMesh = parentMesh;
		if (parentMesh != null) {
			bones = parentMesh.bones;
			vertices = parentMesh.vertices;
			worldVerticesLength = parentMesh.worldVerticesLength;
			regionUVs = parentMesh.regionUVs;
			triangles = parentMesh.triangles;
			hullLength = parentMesh.hullLength;
			edges = parentMesh.edges;
			width = parentMesh.width;
			height = parentMesh.height;
		}
		return _parentMesh;
	}

	override public function copy():Attachment {
		if (parentMesh != null)
			return newLinkedMesh();

		var copy:MeshAttachment = new MeshAttachment(name, this.path);
		copy.region = region;
		copy.color.setFromColor(color);
		copy.rendererObject = rendererObject;

		this.copyTo(copy);
		copy.regionUVs = regionUVs.copy();
		copy.uvs = uvs.copy();
		copy.triangles = triangles.copy();
		copy.hullLength = hullLength;

		copy.sequence = sequence != null ? sequence.copy() : null;

		if (edges != null) {
			copy.edges = edges.copy();
		}
		copy.width = width;
		copy.height = height;

		return copy;
	}

	/** If the attachment has a sequence, the region may be changed. */
	public override function computeWorldVertices(slot:Slot, start:Int, count:Int, worldVertices:Array<Float>, offset:Int, stride:Int):Void {
		if (sequence != null)
			sequence.apply(slot, this);
		super.computeWorldVertices(slot, start, count, worldVertices, offset, stride);
	}

	/** Returns a new mesh with the parentMesh set to this mesh's parent mesh, if any, else to this mesh. */
	public function newLinkedMesh():MeshAttachment {
		var copy:MeshAttachment = new MeshAttachment(name, path);
		copy.rendererObject = rendererObject;
		copy.region = region;
		copy.color.setFromColor(color);
		copy.timelineAttachment = timelineAttachment;
		copy.parentMesh = this.parentMesh != null ? this.parentMesh : this;
		if (copy.region != null)
			copy.updateRegion();
		return copy;
	}
}
