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

/** An attachment whose vertices make up a composite Bezier curve.
 *
 * @see PathConstraint
 * @see https://esotericsoftware.com/spine-paths Paths in the Spine User Guide
 */
class PathAttachment extends VertexAttachment {
	/** The lengths along the path in the setup pose from the start of the path to the end of each Bezier curve. */
	public var lengths:Array<Float>;
	/** If true, the start and end knots are connected. */
	public var closed:Bool = false;
	/** If true, additional calculations are performed to make computing positions along the path more accurate and movement along
	 * the path have a constant speed. */
	public var constantSpeed:Bool = false;
	/** The color of the path as it was in Spine, or a default color if nonessential data was not exported. Paths are not usually
	 * rendered at runtime. */
	public var color:Color = new Color(0, 0, 0, 0);

	public function new(name:String) {
		super(name);
	}

	override public function copy():Attachment {
		var copy:PathAttachment = new PathAttachment(name);
		copyTo(copy);
		copy.lengths = lengths.copy();
		copy.closed = closed;
		copy.constantSpeed = constantSpeed;
		return copy;
	}
}
