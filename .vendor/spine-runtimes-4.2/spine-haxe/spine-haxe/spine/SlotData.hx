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

/** Stores the setup pose for a spine.Slot. */
class SlotData {
	private var _index:Int;
	private var _name:String;
	private var _boneData:BoneData;

	/** The color used to tint the slot's attachment. If SlotData.darkColor is set, this is used as the light color for two
	 * color tinting. */
	public var color:Color = new Color(1, 1, 1, 1);
	/** The dark color used to tint the slot's attachment for two color tinting, or null if two color tinting is not used. The dark
	 * color's alpha is not used. */
	public var darkColor:Color = null;
	/** The name of the attachment that is visible for this slot in the setup pose, or null if no attachment is visible. */
	public var attachmentName:String;
	/** The blend mode for drawing the slot's attachment. */
	public var blendMode:BlendMode = BlendMode.normal;
	/** False if the slot was hidden in Spine and nonessential data was exported. Does not affect runtime rendering. */
	public var visible:Bool = true;

	public function new(index:Int, name:String, boneData:BoneData) {
		if (index < 0)
			throw new SpineException("index must be >= 0.");
		if (name == null)
			throw new SpineException("name cannot be null.");
		if (boneData == null)
			throw new SpineException("boneData cannot be null.");
		_index = index;
		_name = name;
		_boneData = boneData;
	}

	/** The index of the slot in spine.Skeleton.getSlots(). */
	public var index(get, never):Int;

	private function get_index():Int {
		return _index;
	}

	/** The name of the slot, which is unique across all slots in the skeleton. */
	public var name(get, never):String;

	private function get_name():String {
		return _name;
	}

	/** The bone this slot belongs to. */
	public var boneData(get, never):BoneData;

	private function get_boneData():BoneData {
		return _boneData;
	}

	public function toString():String {
		return _name;
	}
}
