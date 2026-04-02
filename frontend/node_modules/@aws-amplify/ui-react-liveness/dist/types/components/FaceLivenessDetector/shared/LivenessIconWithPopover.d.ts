/**
 * Copied from src/primitives/Alert/AlertIcon.tsx because we want to re-use the icon but it is not currently expored by AlertIcon.
 * We currently don't want to make a change to the AlertIcon primitive itself and may expose the icon in the future but for now so as not to introduce cross component dependencies we have duplicated it.
 */
import * as React from 'react';
export interface LivenessIconWithPopoverProps {
    children: string;
    headingText: string;
    labelText: string;
}
export declare const LivenessIconWithPopover: React.FC<LivenessIconWithPopoverProps>;
