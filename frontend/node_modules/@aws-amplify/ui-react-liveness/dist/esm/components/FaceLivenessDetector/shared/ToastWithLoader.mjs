import * as React from 'react';
import { Flex, Loader, View } from '@aws-amplify/ui-react';
import { LivenessClassNames } from '../types/classNames.mjs';
import { Toast } from './Toast.mjs';

const ToastWithLoader = ({ displayText, }) => {
    return (React.createElement(Toast, { "aria-live": "polite" },
        React.createElement(Flex, { className: LivenessClassNames.HintText },
            React.createElement(Loader, null),
            React.createElement(View, null, displayText))));
};

export { ToastWithLoader };
