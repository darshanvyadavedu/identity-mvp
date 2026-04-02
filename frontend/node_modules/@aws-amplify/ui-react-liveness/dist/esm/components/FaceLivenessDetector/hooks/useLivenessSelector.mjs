import { useSelector } from '@xstate/react';
import { useFaceLivenessDetector } from '../providers/FaceLivenessDetectorProvider.mjs';

function createLivenessSelector(selector) {
    return selector;
}
function useLivenessSelector(selector) {
    const { service } = useFaceLivenessDetector();
    return useSelector(service, selector);
}

export { createLivenessSelector, useLivenessSelector };
