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

/** Controls how timeline values are mixed with setup pose values or current pose values when a timeline is applied with
 * alpha < 1.
 * 
 * @see spine.animation.Timeline.apply() */
class MixBlend {
	public var ordinal:Int = 0;

	public function new(ordinal:Int) {
		this.ordinal = ordinal;
	}

	/** Transitions from the setup value to the timeline value (the current value is not used). Before the first frame, the
	 * setup value is set. */
	public static var setup(default, never):MixBlend = new MixBlend(0);
	/** Transitions from the current value to the timeline value. Before the first frame, transitions from the current value to
	 * the setup value. Timelines which perform instant transitions, such as spine.animation.DrawOrderTimeline or
	 * spine.animation.AttachmentTimeline, use the setup value before the first frame.
	 * 
	 * first is intended for the first animations applied, not for animations layered on top of those. */
	public static var first(default, never):MixBlend = new MixBlend(1);
	/** Transitions from the current value to the timeline value. No change is made before the first frame (the current value is
	 * kept until the first frame).
	 * 
	 * replace is intended for animations layered on top of others, not for the first animations applied. */
	public static var replace(default, never):MixBlend = new MixBlend(2);
	/** Transitions from the current value to the current value plus the timeline value. No change is made before the first
	 * frame (the current value is kept until the first frame).
	 * 
	 * add is intended for animations layered on top of others, not for the first animations applied. Properties
	 * set by additive animations must be set manually or by another animation before applying the additive animations, else the
	 * property values will increase each time the additive animations are applied. */
	public static var add(default, never):MixBlend = new MixBlend(3);
}
