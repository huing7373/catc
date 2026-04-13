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

package spine.animation;

import haxe.ds.StringMap;
import spine.SkeletonData;

/** Stores mix (crossfade) durations to be applied when spine.animation.AnimationState animations are changed. */
class AnimationStateData {
	private var _skeletonData:SkeletonData;
	private var animationToMixTime:StringMap<Float> = new StringMap<Float>();

	/** The mix duration to use when no mix duration has been defined between two animations. */
	public var defaultMix:Float = 0;

	public function new(skeletonData:SkeletonData) {
		_skeletonData = skeletonData;
	}

	/** The SkeletonData to look up animations when they are specified by name. */
	public var skeletonData(get, never):SkeletonData;

	private function get_skeletonData():SkeletonData {
		return _skeletonData;
	}

	/** Sets a mix duration by animation name.
	 * 
	 * See AnimationStateData.setMix(). */
	public function setMixByName(fromName:String, toName:String, duration:Float):Void {
		var from:Animation = _skeletonData.findAnimation(fromName);
		if (from == null)
			throw new SpineException("Animation not found: " + fromName);
		var to:Animation = _skeletonData.findAnimation(toName);
		if (to == null)
			throw new SpineException("Animation not found: " + toName);
		setMix(from, to, duration);
	}

	/** Sets the mix duration when changing from the specified animation to the other.
	 * 
	 * See spine.animation.TrackEntry.mixDuration. */
	public function setMix(from:Animation, to:Animation, duration:Float):Void {
		if (from == null)
			throw new SpineException("from cannot be null.");
		if (to == null)
			throw new SpineException("to cannot be null.");
		animationToMixTime.set(from.name + ":" + to.name, duration);
	}

	/** Returns the mix duration to use when changing from the specified animation to the other, or the AnimationStateData.defaultMix if
	 * no mix duration has been set. */
	public function getMix(from:Animation, to:Animation):Float {
		if (animationToMixTime.exists(from.name + ":" + to.name))
			return animationToMixTime.get(from.name + ":" + to.name);
		else
			return defaultMix;
	}
}
