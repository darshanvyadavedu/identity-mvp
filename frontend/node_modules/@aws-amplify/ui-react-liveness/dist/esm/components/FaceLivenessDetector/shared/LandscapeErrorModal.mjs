import * as React from 'react';
import { Flex, Text, Button } from '@aws-amplify/ui-react';
import { getLandscapeMediaQuery } from '../utils/device.mjs';
import { LivenessClassNames } from '../types/classNames.mjs';

const LandscapeErrorModal = (props) => {
    const { onRetry, header, portraitMessage, landscapeMessage, tryAgainText } = props;
    const [isLandscape, setIsLandscape] = React.useState(true);
    React.useLayoutEffect(() => {
        // Get orientation: landscape media query
        const landscapeMediaQuery = getLandscapeMediaQuery();
        // Set ui state for initial orientation
        setIsLandscape(landscapeMediaQuery.matches);
        // Listen for future orientation changes
        landscapeMediaQuery.addEventListener('change', (e) => {
            setIsLandscape(e.matches);
        });
        // Remove matchMedia event listener
        return () => {
            landscapeMediaQuery.removeEventListener('change', (e) => setIsLandscape(e.matches));
        };
    }, []);
    return (React.createElement(Flex, { className: LivenessClassNames.LandscapeErrorModal, height: isLandscape ? 'auto' : 480 },
        React.createElement(Text, { className: LivenessClassNames.LandscapeErrorModalHeader }, header),
        React.createElement(Text, null, isLandscape ? landscapeMessage : portraitMessage),
        !isLandscape ? (React.createElement(Flex, { className: LivenessClassNames.LandscapeErrorModalButton },
            React.createElement(Button, { variation: "primary", type: "button", onClick: onRetry }, tryAgainText))) : null));
};

export { LandscapeErrorModal };
