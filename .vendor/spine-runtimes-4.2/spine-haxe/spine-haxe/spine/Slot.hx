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

package spine;

import spine.attachments.Attachment;
import spine.attachments.VertexAttachment;

/** Stores a slot's current pose. Slots organize attachments for Skeleton.drawOrder purposes and provide a place to store
 * state for an attachment. State cannot be stored in an attachment itself because attachments are stateless and may be shared
 * across multiple skeletons. */
class Slot {
	private var _data:SlotData;
	private var _bone:Bone;

	/** The color used to tint the slot's attachment. If darkColor is set, this is used as the light color for two
	 * color tinting. */
	public var color:Color;
	/** The dark color used to tint the slot's attachment for two color tinting, or null if two color tinting is not used. The dark
	 * color's alpha is not used. */
	public var darkColor:Color;

	private var _attachment:Attachment;

	/** The index of the texture region to display when the slot's attachment has a spine.attachments.Sequence. -1 represents the
	 * Sequence.getSetupIndex(). */
	public var sequenceIndex = -1;

	public var attachmentState:Int = 0;
	/** Values to deform the slot's attachment. For an unweighted mesh, the entries are local positions for each vertex. For a
	 * weighted mesh, the entries are an offset for each vertex which will be added to the mesh's local vertex positions.
	 * @see spine.attachments.VertexAttachment.computeWorldVertices()
	 * @see spine.animation.DeformTimeline */
	public var deform:Array<Float> = new Array<Float>();

	/** Copy constructor. */
	public function new(data:SlotData, bone:Bone) {
		if (data == null)
			throw new SpineException("data cannot be null.");
		if (bone == null)
			throw new SpineException("bone cannot be null.");
		_data = data;
		_bone = bone;
		this.color = new Color(1, 1, 1, 1);
		this.darkColor = data.darkColor == null ? null : new Color(1, 1, 1, 1);
		setToSetupPose();
	}

	/** The slot's setup pose data. */
	public var data(get, never):SlotData;

	private function get_data():SlotData {
		return _data;
	}

	/** The bone this slot belongs to. */
	public var bone(get, never):Bone;

	private function get_bone():Bone {
		return _bone;
	}

	/** The skeleton this slot belongs to. */
	public var skeleton(get, never):Skeleton;

	private function get_skeleton():Skeleton {
		return _bone.skeleton;
	}

	/** The current attachment for the slot, or null if the slot has no attachment. */
	public var attachment(get, set):Attachment;

	private function get_attachment():Attachment {
		return _attachment;
	}

	/** Sets the slot's attachment and, if the attachment changed, resets sequenceIndex and clears the deform.
	 * The deform is not cleared if the old attachment has the same spine.attachments.VertexAttachment.timelineAttachment as the
	 * specified attachment. */
	public function set_attachment(attachmentNew:Attachment):Attachment {
		if (attachment == attachmentNew)
			return attachmentNew;
		if (!Std.isOfType(attachmentNew, VertexAttachment)
			|| !Std.isOfType(attachment, VertexAttachment)
			|| cast(attachmentNew, VertexAttachment).timelineAttachment != cast(attachment, VertexAttachment).timelineAttachment) {
			deform = new Array<Float>();
		}
		_attachment = attachmentNew;
		sequenceIndex = -1;
		return attachmentNew;
	}

	/** Sets this slot to the setup pose. */
	public function setToSetupPose():Void {
		color.setFromColor(data.color);
		if (darkColor != null)
			darkColor.setFromColor(data.darkColor);
		if (_data.attachmentName == null) {
			attachment = null;
		} else {
			_attachment = null;
			attachment = skeleton.getAttachmentForSlotIndex(data.index, data.attachmentName);
		}
	}

	public function toString():String {
		return _data.name != null ? _data.name : "Slot?";
	}
}
