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

import { AnimationsInfo, FitType, OffScreenUpdateBehaviourType } from "./SpineWebComponentSkeleton.js";

const animatonTypeRegExp = /\[([^\]]+)\]/g;
export type AttributeTypes = "string" | "number" | "boolean" | "array-number" | "array-string" | "object" | "fitType" | "offScreenUpdateBehaviourType" | "animationsInfo";

export function castValue (type: AttributeTypes, value: string | null, defaultValue?: any) {
	switch (type) {
		case "string":
			return castString(value, defaultValue);
		case "number":
			return castNumber(value, defaultValue);
		case "boolean":
			return castBoolean(value, defaultValue);
		case "array-number":
			return castArrayNumber(value, defaultValue);
		case "array-string":
			return castArrayString(value, defaultValue);
		case "object":
			return castObject(value, defaultValue);
		case "fitType":
			return isFitType(value) ? value : defaultValue;
		case "offScreenUpdateBehaviourType":
			return isOffScreenUpdateBehaviourType(value) ? value : defaultValue;
		case "animationsInfo":
			return castToAnimationsInfo(value) || defaultValue;
		default:
			break;
	}
}

function castBoolean (value: string | null, defaultValue = "") {
	return value === "true" || value === "" ? true : false;
}

function castString (value: string | null, defaultValue = "") {
	return value === null ? defaultValue : value;
}

function castNumber (value: string | null, defaultValue = 0) {
	if (value === null) return defaultValue;

	const parsed = parseFloat(value);
	if (Number.isNaN(parsed)) return defaultValue;
	return parsed;
}

function castArrayNumber (value: string | null, defaultValue = undefined) {
	if (value === null) return defaultValue;
	return value.split(",").reduce((acc, pageIndex) => {
		const index = parseInt(pageIndex);
		if (!isNaN(index)) acc.push(index);
		return acc;
	}, [] as Array<number>);
}

function castArrayString (value: string | null, defaultValue = undefined) {
	if (value === null) return defaultValue;
	return value.split(",");
}

function castObject (value: string | null, defaultValue = undefined) {
	if (value === null) return null;
	return JSON.parse(value);
}


function castToAnimationsInfo (value: string | null): AnimationsInfo | undefined {
	if (value === null) {
		return undefined;
	}

	const matches = value.match(animatonTypeRegExp);
	if (!matches) return undefined;

	return matches.reduce((obj, group) => {
		const [trackIndexStringOrLoopDefinition, animationNameOrTrackIndexStringCycle, loopOrRepeatDelay, delayString, mixDurationString] = group.slice(1, -1).split(',').map(v => v.trim());

		if (trackIndexStringOrLoopDefinition === "loop") {
			if (!Number.isInteger(Number(animationNameOrTrackIndexStringCycle))) {
				throw new Error(`Track index of cycle in ${group} must be a positive integer number, instead it is ${animationNameOrTrackIndexStringCycle}. Original value: ${value}`);
			}
			const animationInfoObject = obj[animationNameOrTrackIndexStringCycle] ||= { animations: [] };
			animationInfoObject.cycle = true;

			if (loopOrRepeatDelay !== undefined) {
				const repeatDelay = Number(loopOrRepeatDelay);
				if (Number.isNaN(repeatDelay)) {
					throw new Error(`If present, duration of last animation of cycle in ${group} must be a positive integer number, instead it is ${loopOrRepeatDelay}. Original value: ${value}`);
				}
				animationInfoObject.repeatDelay = repeatDelay;
			}

			return obj;
		}

		const trackIndex = Number(trackIndexStringOrLoopDefinition);
		if (!Number.isInteger(trackIndex)) {
			throw new Error(`Track index in ${group} must be a positive integer number, instead it is ${trackIndexStringOrLoopDefinition}. Original value: ${value}`);
		}

		let delay;
		if (delayString !== undefined) {
			delay = parseFloat(delayString);
			if (isNaN(delay)) {
				throw new Error(`Delay in ${group} must be a positive number, instead it is ${delayString}. Original value: ${value}`);
			}
		}

		let mixDuration;
		if (mixDurationString !== undefined) {
			mixDuration = parseFloat(mixDurationString);
			if (isNaN(mixDuration)) {
				throw new Error(`mixDuration in ${group} must be a positive number, instead it is ${mixDurationString}. Original value: ${value}`);
			}
		}

		const animationInfoObject = obj[trackIndexStringOrLoopDefinition] ||= { animations: [] };
		animationInfoObject.animations.push({
			animationName: animationNameOrTrackIndexStringCycle,
			loop: (loopOrRepeatDelay || "").trim().toLowerCase() === "true",
			delay,
			mixDuration,
		});
		return obj;
	}, {} as AnimationsInfo);
}

function isFitType (value: string | null): value is FitType {
	return (
		value === "fill" ||
		value === "width" ||
		value === "height" ||
		value === "contain" ||
		value === "cover" ||
		value === "none" ||
		value === "scaleDown" ||
		value === "origin"
	);
}

function isOffScreenUpdateBehaviourType (value: string | null): value is OffScreenUpdateBehaviourType {
	return (
		value === "pause" ||
		value === "update" ||
		value === "pose"
	);
}

const base64RegExp = /^(([A-Za-z0-9+/]{4})*([A-Za-z0-9+/]{4}|[A-Za-z0-9+/]{3}=|[A-Za-z0-9+/]{2}==))$/;
export function isBase64 (str: string) {
	return base64RegExp.test(str);
}

export interface Point {
	x: number,
	y: number,
}

export interface Rectangle extends Point {
	width: number,
	height: number,
}