import React__default from 'react';
import { Flex, Icon, Text } from '@aws-amplify/ui-react';
import { LivenessClassNames } from '../types/classNames.mjs';

const RecordingIcon = ({ children }) => {
    return (React__default.createElement(Flex, { className: LivenessClassNames.RecordingIcon },
        React__default.createElement(Flex, { "data-testid": "rec-icon", justifyContent: "center" },
            React__default.createElement(Icon, { viewBox: { width: 20, height: 20 }, width: "20", height: "20" },
                React__default.createElement("circle", { cx: "10", cy: "10", r: "8", fill: "red" }))),
        React__default.createElement(Text, { as: "span", fontWeight: "bold" }, children)));
};

export { RecordingIcon };
