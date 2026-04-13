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

/** Stores the setup pose for a spine.Bone. */
class BoneData {
	private var _index:Int;
	private var _name:String;
	private var _parent:BoneData;

	/** The bone's length. */
	public var length:Float = 0;
	/** The local x translation. */
	public var x:Float = 0;
	/** The local y translation. */
	public var y:Float = 0;
	/** The local rotation in degrees, counter clockwise. */
	public var rotation:Float = 0;
	/** The local scaleX. */
	public var scaleX:Float = 1;
	/** The local scaleY. */
	public var scaleY:Float = 1;
	/** The local shearX. */
	public var shearX:Float = 0;
	/** The local shearY. */
	public var shearY:Float = 0;
	/** Determines how parent world transforms affect this bone. */
	public var inherit:Inherit = Inherit.normal;
	/** When true, spine.Skeleton.updateWorldTransform() only updates this bone if the spine.Skeleton.getSkin() contains
	 * this bone.
	 * @see spine.Skin.getBones() */
	public var skinRequired:Bool = false;
	/** The color of the bone as it was in Spine, or a default color if nonessential data was not exported. Bones are not usually
	 * rendered at runtime. */
	public var color:Color = new Color(0, 0, 0, 0);
	/** The bone icon as it was in Spine, or null if nonessential data was not exported. */
	public var icon:String;
	/** False if the bone was hidden in Spine and nonessential data was exported. Does not affect runtime rendering. */
	public var visible:Bool = false;

	/** Copy constructor. */
	public function new(index:Int, name:String, parent:BoneData) {
		if (index < 0)
			throw new SpineException("index must be >= 0");
		if (name == null)
			throw new SpineException("name cannot be null.");
		_index = index;
		_name = name;
		_parent = parent;
	}

	/** The index of the bone in spine.Skeleton.getBones(). */
	public var index(get, never):Int;

	private function get_index():Int {
		return _index;
	}

	/** The name of the bone, which is unique across all bones in the skeleton. */
	public var name(get, never):String;

	private function get_name():String {
		return _name;
	}

	/** @return May be null. */
	public var parent(get, never):BoneData;

	private function get_parent():BoneData {
		return _parent;
	}

	public function toString():String {
		return _name;
	}
}
