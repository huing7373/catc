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

import spine.Bone;
import spine.Color;
import spine.MathUtils;

/** An attachment which is a single point and a rotation. This can be used to spawn projectiles, particles, etc. A bone can be
 * used in similar ways, but a PointAttachment is slightly less expensive to compute and can be hidden, shown, and placed in a
 * skin.
 *
 * @see https://esotericsoftware.com/spine-points Point Attachments in the Spine User Guide */
class PointAttachment extends VertexAttachment {
	public var x:Float = 0;
	public var y:Float = 0;
	public var rotation:Float = 0;
	/** The color of the point attachment as it was in Spine, or a default color if nonessential data was not exported. Point
	 * attachments are not usually rendered at runtime. */
	public var color:Color = new Color(0.38, 0.94, 0, 1);

	/** Copy constructor. */
	public function new(name:String) {
		super(name);
	}

	public function computeWorldPosition(bone:Bone, point:Array<Float>):Array<Float> {
		point[0] = x * bone.a + y * bone.b + bone.worldX;
		point[1] = x * bone.c + y * bone.d + bone.worldY;
		return point;
	}

	public function computeWorldRotation(bone:Bone):Float {
		var r:Float = this.rotation * MathUtils.degRad, cos:Float = Math.cos(r), sin:Float = Math.sin(r);
		var x:Float = cos * bone.a + sin * bone.b;
		var y:Float = cos * bone.c + sin * bone.d;
		return MathUtils.atan2Deg(y, x);
	}

	override public function copy():Attachment {
		var copy:PointAttachment = new PointAttachment(name);
		copy.x = x;
		copy.y = y;
		copy.rotation = rotation;
		copy.color.setFromColor(color);
		return copy;
	}
}
