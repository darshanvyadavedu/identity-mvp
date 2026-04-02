import React__default from 'react';

const FaceLivenessDetectorContext = React__default.createContext(null);
function FaceLivenessDetectorProvider({ children, ...props }) {
    return (React__default.createElement(FaceLivenessDetectorContext.Provider, { value: props }, children));
}
function useFaceLivenessDetector() {
    const props = React__default.useContext(FaceLivenessDetectorContext);
    if (props === null) {
        throw new Error('useFaceLivenessDetector must be used within a FaceLivenessDetectorProvider');
    }
    return props;
}

export { FaceLivenessDetectorProvider, useFaceLivenessDetector };
