import * as React from 'react';
import { useTheme, View, Flex } from '@aws-amplify/ui-react';
import { LivenessClassNames } from '../types/classNames.mjs';

const Toast = ({ variation = 'default', size = 'medium', children, isInitial = false, ...rest }) => {
    const { tokens } = useTheme();
    return (React.createElement(View, { className: `${LivenessClassNames.Toast} ${LivenessClassNames.Toast}--${variation} ${LivenessClassNames.Toast}--${size}`, ...(isInitial && { backgroundColor: tokens.colors.background.primary }), ...rest },
        React.createElement(Flex, { className: LivenessClassNames.ToastContainer },
            React.createElement(Flex, { className: LivenessClassNames.ToastMessage, ...(isInitial ? { color: tokens.colors.font.primary } : {}) }, children))));
};

export { Toast };
