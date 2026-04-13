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

/**
 * Stores the setup pose for a spine.TransformConstraint.
 * 
 * 
 * @see https://esotericsoftware.com/spine-transform-constraints Transform constraints in the Spine User Guide
 */
class TransformConstraintData extends ConstraintData {
	private var _bones:Array<BoneData> = new Array<BoneData>();

	/** The target bone whose world transform will be copied to the constrained bones. */
	public var target:BoneData;
	/** A percentage (0-1) that controls the mix between the constrained and unconstrained rotation. */
	public var mixRotate:Float = 0;
	/** A percentage (0-1) that controls the mix between the constrained and unconstrained translation X. */
	public var mixX:Float = 0;
	/** A percentage (0-1) that controls the mix between the constrained and unconstrained translation Y. */
	public var mixY:Float = 0;
	/** A percentage (0-1) that controls the mix between the constrained and unconstrained scale X. */
	public var mixScaleX:Float = 0;
	/** A percentage (0-1) that controls the mix between the constrained and unconstrained scale Y. */
	public var mixScaleY:Float = 0;
	/** A percentage (0-1) that controls the mix between the constrained and unconstrained shear Y. */
	public var mixShearY:Float = 0;
	/** An offset added to the constrained bone rotation. */
	public var offsetRotation:Float = 0;
	/** An offset added to the constrained bone X translation. */
	public var offsetX:Float = 0;
	/** An offset added to the constrained bone Y translation. */
	public var offsetY:Float = 0;
	/** An offset added to the constrained bone scaleX. */
	public var offsetScaleX:Float = 0;
	/** An offset added to the constrained bone scaleY. */
	public var offsetScaleY:Float = 0;
	/** An offset added to the constrained bone shearY. */
	public var offsetShearY:Float = 0;
	public var relative:Bool = false;
	public var local:Bool = false;

	public function new(name:String) {
		super(name, 0, false);
	}

	/**
	 * The bones that will be modified by this transform constraint.
	 */
	public var bones(get, never):Array<BoneData>;

	private function get_bones():Array<BoneData> {
		return _bones;
	}
}
