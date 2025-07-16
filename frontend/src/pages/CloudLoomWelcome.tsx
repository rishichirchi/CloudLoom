import { Button } from "@/components/ui/button";
import { useNavigate } from "react-router-dom";

const CloudLoomWelcome = () => {
  const navigate = useNavigate();
  return (
    <div className="flex flex-col items-center justify-center min-h-screen bg-gradient-to-br from-blue-100 to-indigo-200 relative overflow-hidden">
      {/* Decorative texture */}
      <div className="absolute inset-0 pointer-events-none z-0" style={{
        backgroundImage: `url('https://www.transparenttextures.com/patterns/cubes.png')`,
        opacity: 0.15,
        mixBlendMode: 'multiply',
      }} />
      <div className="text-center z-10 relative">
        <h1 className="text-5xl font-extrabold text-indigo-700 mb-4 drop-shadow-lg">CloudLoom</h1>
        <p className="text-lg text-gray-700 mb-4 max-w-xl mx-auto">
          Welcome to <span className="font-bold text-indigo-600">CloudLoom</span>, your intelligent cloud security assistant. Effortlessly monitor, assess, and secure your cloud infrastructure with AI-powered insights and automated fixes. Start your journey to a safer cloud today.
        </p>
        <Button size="lg" className="px-8 py-3 text-lg font-semibold" onClick={() => navigate("/choose-plan")}>Get Started</Button>
      </div>
    </div>
  );
};

export default CloudLoomWelcome;
