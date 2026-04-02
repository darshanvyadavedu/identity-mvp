import React__default from 'react';
import { Button } from '@aws-amplify/ui-react';
import { IconClose } from '@aws-amplify/ui-react/internal';
import { useLivenessActor } from '../hooks/useLivenessActor.mjs';
import '@xstate/react';
import '../providers/FaceLivenessDetectorProvider.mjs';
import '@aws-amplify/ui';
import { LivenessClassNames } from '../types/classNames.mjs';

const CancelButton = ({ ariaLabel }) => {
    const [state, send] = useLivenessActor();
    const isFinalState = state.done;
    const handleClick = () => {
        send({
            type: 'CANCEL',
        });
    };
    if (isFinalState)
        return null;
    return (React__default.createElement(Button, { autoFocus: true, variation: "link", onClick: handleClick, size: "large", className: LivenessClassNames.CancelButton, "aria-label": ariaLabel },
        React__default.createElement(IconClose, { "aria-hidden": "true", "data-testid": "close-icon" })));
};

export { CancelButton };
